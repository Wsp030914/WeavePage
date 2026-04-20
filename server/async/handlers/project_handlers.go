package handlers

import (
	"ToDoList/server/async"
	"ToDoList/server/models"
	"context"
	"encoding/json"

	"go.uber.org/zap"
)

type GetProjectSummaryPayload = models.ProjectSummaryCache

func PutProjectsSummary(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var summary GetProjectSummaryPayload
	if err := json.Unmarshal(job.Payload, &summary); err != nil {
		lg.Error(job.Type+" payload unmarshal failed", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}

	if summary.UID <= 0 {
		lg.Error(job.Type+" invalid payload: uid <= 0", zap.String("trace_id", job.TraceID))
		return nil
	}
	if summary.Page <= 0 {
		summary.Page = 1
	}
	if summary.Size <= 0 {
		summary.Size = 20
	}

	if globalDeps.ProjectCache == nil {
		return nil
	}

	if err := globalDeps.ProjectCache.SetSummary(ctx, summary); err != nil {
		lg.Error(job.Type+" set summary cache failed", zap.Int("uid", summary.UID), zap.Error(err))
		return err
	}

	if len(summary.Projects) > 0 {
		if err := globalDeps.ProjectCache.MSet(ctx, summary.UID, summary.Projects); err != nil {
			lg.Error(job.Type+" warm project detail cache failed", zap.Int("uid", summary.UID), zap.Error(err))
			return err
		}
	}

	lg.Info(job.Type+" executed",
		zap.Int("uid", summary.UID),
		zap.String("name", summary.Name),
		zap.Int("page", summary.Page),
		zap.Int("size", summary.Size),
	)
	return nil
}
