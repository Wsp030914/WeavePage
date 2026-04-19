package handlers

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	"ToDoList/server/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type cosDeletePayload struct {
	Key string `json:"key"`
}
type avatarKeyPut struct {
	UID       int    `json:"uid"`
	AvatarKey string `json:"avatarKey"`
	AvatarURL string `json:"avatarURL"`
}

type avatarKeyDel struct {
	UID int `json:"uid"`
}

type putVersion struct {
	UID          int `json:"uid"`
	TokenVersion int `json:"tokenVersion"`
}

func avatarKeyCacheKey(uid int) string {
	return fmt.Sprintf("user:avatar:key:%d", uid)
}

func DeleteCosObject(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var p cosDeletePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		lg.Error(job.Type+" payload unmarshal failed", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}
	key := utils.NormalizeObjectKey(p.Key)
	if key == "" {
		lg.Error(job.Type+" invalid payload: empty key", zap.String("trace_id", job.TraceID))
		return nil
	}
	err := utils.DeleteObject(ctx, key)
	if err != nil {
		lg.Error(job.Type+" delete cos object failed", zap.String("key", key), zap.Error(err))
		return err
	}
	lg.Info(job.Type+" executed", zap.String("key", key))
	return err
}

func UpdateAvatarKey(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var a avatarKeyPut
	if err := json.Unmarshal(job.Payload, &a); err != nil {
		lg.Error(job.Type+" payload unmarshal failed", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}
	if a.UID <= 0 || a.AvatarKey == "" {
		lg.Error(job.Type+" invalid payload", zap.Int("uid", a.UID), zap.String("trace_id", job.TraceID))
		return nil
	}
	avatarKey := utils.NormalizeObjectKey(a.AvatarKey)
	if avatarKey == "" {
		lg.Error(job.Type+" invalid payload: empty normalized avatar key", zap.Int("uid", a.UID), zap.String("trace_id", job.TraceID))
		return nil
	}
	avatarURL := strings.TrimSpace(a.AvatarURL)
	if avatarURL == "" {
		avatarURL = utils.ObjectURLFromKey(avatarKey)
	}

	// If profile is cached, patch avatar_url in place.
	if globalDeps.UserCache != nil {
		profile, err := globalDeps.UserCache.GetProfile(ctx, a.UID)
		switch {
		case err == nil && profile != nil:
			profile.AvatarURL = avatarURL
			if setErr := globalDeps.UserCache.SetProfile(ctx, a.UID, profile); setErr != nil {
				lg.Error(job.Type+" set profile cache failed", zap.Int("uid", a.UID), zap.Error(setErr))
				return setErr
			}
		case err != nil:
			if !errors.Is(err, redis.Nil) && !errors.Is(err, cache.ErrCacheNotFound) {
				lg.Error(job.Type+" get profile cache failed", zap.Int("uid", a.UID), zap.Error(err))
				return err
			}
		}
	}

	// Also cache raw avatar key for lightweight reads/debug.
	if globalDeps.Cache != nil {
		if err := globalDeps.Cache.Set(ctx, avatarKeyCacheKey(a.UID), avatarKey, 24*time.Hour); err != nil {
			lg.Error(job.Type+" set avatar key cache failed", zap.Int("uid", a.UID), zap.Error(err))
			return err
		}
	}

	lg.Info(job.Type+" executed", zap.Int("uid", a.UID), zap.String("avatar_key", avatarKey), zap.String("avatar_url", avatarURL))
	return nil
}

func PutVersion(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var p putVersion
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		lg.Error(job.Type+" payload unmarshal failed", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}
	if p.UID <= 0 {
		lg.Error(job.Type+" invalid payload: uid <= 0", zap.String("trace_id", job.TraceID))
		return nil
	}
	if p.TokenVersion <= 0 {
		lg.Error(job.Type+" invalid payload: token_version <= 0", zap.Int("uid", p.UID), zap.String("trace_id", job.TraceID))
		return nil
	}

	if globalDeps.UserCache == nil {
		lg.Warn(job.Type+" skipped: user cache dependency is nil", zap.Int("uid", p.UID))
		return nil
	}
	if err := globalDeps.UserCache.SetVersion(ctx, p.UID, p.TokenVersion); err != nil {
		lg.Error(job.Type+" set version cache failed", zap.Int("uid", p.UID), zap.Int("version", p.TokenVersion), zap.Error(err))
		return err
	}

	lg.Info(job.Type+" executed", zap.Int("uid", p.UID), zap.Int("version", p.TokenVersion))
	return nil
}
