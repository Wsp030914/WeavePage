package service

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/repo"
	"ToDoList/server/utils"
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthService struct {
	repo      repo.UserRepository
	userCache cache.UserCache
	bus       async.IEventBus
}

type AuthServiceDeps struct {
	Repo      repo.UserRepository
	UserCache cache.UserCache
	Bus       async.IEventBus
}

func NewAuthService(deps AuthServiceDeps) *AuthService {
	return &AuthService{
		repo:      deps.Repo,
		userCache: deps.UserCache,
		bus:       deps.Bus,
	}
}

// ValidateClaims validates access-token revocation and token-version state.
func (a *AuthService) ValidateClaims(ctx context.Context, lg *zap.Logger, claims *utils.Claims) error {
	if claims == nil {
		return apperrors.NewUnauthorizedError("token invalid")
	}
	if err := a.ValidateJti(ctx, lg, claims.RegisteredClaims.ID); err != nil {
		return err
	}
	return a.ValidateVersion(ctx, lg, claims.UID, claims.Ver)
}

func (a *AuthService) ValidateJti(ctx context.Context, lg *zap.Logger, jti string) error {
	blacklisted, err := a.userCache.GetJti(ctx, jti)
	if err != nil && err != redis.Nil {
		lg.Warn("user.auth.GetJti_redis_failed", zap.Error(err))
		return apperrors.NewInternalError("service busy")
	}
	if blacklisted {
		return apperrors.NewUnauthorizedError("user logged out")
	}
	return nil
}

func (a *AuthService) ValidateVersion(ctx context.Context, lg *zap.Logger, uid int, reqVersion int) error {
	version, cacheErr := a.userCache.GetVersion(ctx, uid)
	if cacheErr == nil {
		if reqVersion == version {
			return nil
		}
		return apperrors.NewUnauthorizedError("token invalid")
	}

	if cacheErr != redis.Nil {
		lg.Warn("user.auth.GetVersion_redis_failed", zap.Int("uid", uid), zap.Error(cacheErr))
	}

	u, dbErr := a.repo.GetVersion(ctx, uid)
	if dbErr != nil {
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("user not found")
		}
		return apperrors.NewInternalError("service busy")
	}
	if reqVersion != u.TokenVersion {
		return apperrors.NewUnauthorizedError("token invalid")
	}

	if setErr := a.userCache.SetVersion(ctx, u.ID, u.TokenVersion); setErr != nil {
		lg.Warn("user.auth.SetVersion_redis_failed", zap.Int("uid", u.ID), zap.Error(setErr))
		if a.bus != nil {
			async.PublishWithTimeout(a.bus, lg, "PutVersion", struct {
				UID          int `json:"uid"`
				TokenVersion int `json:"tokenVersion"`
			}{UID: u.ID, TokenVersion: u.TokenVersion}, 100*time.Millisecond, zap.Int("uid", u.ID))
		}
	}
	return nil
}
