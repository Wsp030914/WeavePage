package handlers

import (
	"ToDoList/server/async"
	"ToDoList/server/models"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type GetProjectSummaryPayload struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
	UID   int         `json:"uid"`
	Ver   int64       `json:"ver"`
	Name  string      `json:"name"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func projectSummaryKey(uid int, ver int64, name string, page, size int) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(name)))
	return fmt.Sprintf("project:summary:%d:%d:%08x:%d:%d", uid, ver, h.Sum32(), page, size)
}

func decodeProjectItems(items any) ([]models.Project, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var projects []models.Project
	if err := json.Unmarshal(raw, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func PutProjectsSummary(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var g GetProjectSummaryPayload
	if err := json.Unmarshal(job.Payload, &g); err != nil {
		lg.Error(job.Type+" payload unmarshal failed", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}

	if g.UID <= 0 {
		lg.Error(job.Type+" invalid payload: uid <= 0", zap.String("trace_id", job.TraceID))
		return nil
	}
	if g.Page <= 0 {
		g.Page = 1
	}
	if g.Size <= 0 {
		g.Size = 20
	}

	// Persist project summary snapshot to redis.
	if globalDeps.Cache != nil {
		key := projectSummaryKey(g.UID, g.Ver, g.Name, g.Page, g.Size)
		raw, err := json.Marshal(g)
		if err != nil {
			lg.Error(job.Type+" marshal summary payload failed", zap.Int("uid", g.UID), zap.Error(err))
			return err
		}
		if err := globalDeps.Cache.Set(ctx, key, string(raw), 15*time.Minute); err != nil {
			lg.Error(job.Type+" set summary cache failed", zap.Int("uid", g.UID), zap.Error(err))
			return err
		}
	}

	// Warm project detail cache for this page when payload items are parsable.
	if globalDeps.ProjectCache != nil {
		projects, err := decodeProjectItems(g.Items)
		if err == nil && len(projects) > 0 {
			if setErr := globalDeps.ProjectCache.MSet(ctx, g.UID, projects); setErr != nil {
				lg.Error(job.Type+" warm project detail cache failed", zap.Int("uid", g.UID), zap.Error(setErr))
				return setErr
			}
		} else if err != nil {
			lg.Warn(job.Type+" decode items skipped", zap.Int("uid", g.UID), zap.Error(err))
		}
	}

	lg.Info(job.Type+" executed", zap.Int("uid", g.UID), zap.String("name", g.Name), zap.Int("page", g.Page), zap.Int("size", g.Size))
	return nil
}
