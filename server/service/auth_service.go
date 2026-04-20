package service

// 文件说明：这个文件实现访问令牌校验相关业务。
// 实现方式：把黑名单校验、token 版本校验以及缓存回填收口到单独的认证服务中。
// 这样做的好处是 JWT 鉴权逻辑集中，后续无论切换缓存策略还是补更多鉴权状态都不会污染 handler 和 middleware。

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

// NewAuthService 创建认证服务。
func NewAuthService(deps AuthServiceDeps) *AuthService {
	return &AuthService{
		repo:      deps.Repo,
		userCache: deps.UserCache,
		bus:       deps.Bus,
	}
}

// ValidateClaims 统一校验访问令牌的黑名单状态和 token 版本状态。
func (a *AuthService) ValidateClaims(ctx context.Context, lg *zap.Logger, claims *utils.Claims) error {
	if claims == nil {
		return apperrors.NewUnauthorizedError("token invalid")
	}
	if err := a.ValidateJti(ctx, lg, claims.RegisteredClaims.ID); err != nil {
		return err
	}
	return a.ValidateVersion(ctx, lg, claims.UID, claims.Ver)
}

// ValidateJti 检查当前 token 是否已被登出拉黑。
// 把登出态放进缓存黑名单，而不是落数据库，是为了让校验链路足够快且与 token 过期时间自然对齐。
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

// ValidateVersion 校验 token_version 是否与服务端一致。
// 这里优先读 Redis，miss 后再查数据库并回填缓存，是为了让密码修改、强制下线这类场景既准确又不把所有请求都压回数据库。
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
