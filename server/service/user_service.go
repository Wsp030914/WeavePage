package service

// 文件说明：这个文件实现用户账号相关业务，包括登录、注册、资料读取、退出登录和个人资料更新。
// 实现方式：服务层组合 repo、缓存、事件总线与对象存储能力，统一编排用户数据的一致性和鉴权辅助状态。
// 这样做的好处是账号规则集中，缓存失效、token 版本和头像资源更新可以放在同一条业务链路里维护。

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"ToDoList/server/utils"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type UserService struct {
	repo         repo.UserRepository
	userCache    cache.UserCache
	cacheClient  cache.Cache
	bus          async.IEventBus
	sf           singleflight.Group
	hashPassword func(password string) (string, error)
	putAvatar    func(ctx context.Context, file *multipart.FileHeader) (string, string, error)
}

type UserServiceDeps struct {
	Repo         repo.UserRepository
	UserCache    cache.UserCache
	CacheClient  cache.Cache
	Bus          async.IEventBus
	HashPassword func(password string) (string, error)
	PutAvatar    func(ctx context.Context, file *multipart.FileHeader) (string, string, error)
}

// NewUserService 创建用户服务，并补齐默认的密码哈希与头像上传实现。
func NewUserService(deps UserServiceDeps) *UserService {
	hashPassword := deps.HashPassword
	if hashPassword == nil {
		hashPassword = utils.HashPassword
	}

	putAvatar := deps.PutAvatar
	if putAvatar == nil {
		putAvatar = utils.PutObj
	}

	return &UserService{
		repo:         deps.Repo,
		userCache:    deps.UserCache,
		cacheClient:  deps.CacheClient,
		bus:          deps.Bus,
		hashPassword: hashPassword,
		putAvatar:    putAvatar,
	}
}

type LoginResult struct {
	AccessToken    string
	AccessExpireAt time.Time
}

// Login 校验账号密码并签发访问令牌。
// 登录阶段直接查身份标识并比对 bcrypt 哈希，是为了让用户名和邮箱入口共用同一条认证逻辑。
func (s *UserService) Login(ctx context.Context, lg *zap.Logger, username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	lg = lg.With(zap.String("username", username))
	lg.Info("login.begin")

	user, err := s.repo.GetByIdentity(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Warn("login.user_not_found")
			return nil, apperrors.NewUnauthorizedError("用户不存在")
		}
		lg.Error("login.query_user_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		lg.Warn("login.password_mismatch")
		return nil, apperrors.NewUnauthorizedError("用户名或密码有误")
	}

	token, exp, err := utils.GenerateAccessToken(user.ID, user.Username, user.TokenVersion)
	if err != nil {
		lg.Error("login.jwt_issue_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("令牌生成失败")
	}
	lg.Info("login.success", zap.Int("uid", user.ID))
	return &LoginResult{
		AccessToken:    token,
		AccessExpireAt: exp,
	}, nil
}

// GetProfile 读取当前用户资料。
// 这里叠加空值缓存、singleflight 和分布式缓存保护，是为了同时防止缓存穿透、击穿和并发回源放大。
func (s *UserService) GetProfile(ctx context.Context, lg *zap.Logger, uid int) (*models.User, error) {
	// 1. 查询缓存
	user, err := s.userCache.GetProfile(ctx, uid)
	if err == nil && user != nil {
		return user, nil
	}
	// 防穿透：判断是否为空值标记
	if errors.Is(err, cache.ErrCacheNotFound) {
		return nil, apperrors.NewNotFoundError("用户不存在")
	}

	key := fmt.Sprintf("user:profile:%d", uid)
	val, err := loadWithCacheProtection(ctx, lg, &s.sf, s.cacheClient, key, func(loadCtx context.Context) (interface{}, error) {
		// 查询数据库
		dbUser, err := s.repo.GetByID(loadCtx, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 防穿透：写入空值
				cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.userCache.SetProfile(cacheCtx, uid, nil)
				return nil, apperrors.NewNotFoundError("用户不存在")
			}
			lg.Error("get_profile.db_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}

		// 3. 回写缓存，确保等待中的跨实例请求能读到结果。
		cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.userCache.SetProfile(cacheCtx, uid, dbUser); err != nil {
			lg.Warn("get_profile.cache_set_failed", zap.Error(err))
		}

		return dbUser, nil
	}, func(readCtx context.Context) (interface{}, bool, error) {
		user, err := s.userCache.GetProfile(readCtx, uid)
		if err == nil && user != nil {
			return user, true, nil
		}
		if errors.Is(err, cache.ErrCacheNotFound) {
			return nil, false, apperrors.NewNotFoundError("用户不存在")
		}
		return nil, false, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(*models.User), nil
}

type RegisterResult struct {
	User models.User
}

// Register 创建新用户。
// 注册时强制头像上传并异步发布头像事件，是为了把账号资料初始化到一个完整状态，同时把后续副作用从主链路拆出去。
func (s *UserService) Register(ctx context.Context, lg *zap.Logger, email, username, password string, avatarFile *multipart.FileHeader) (*RegisterResult, error) {
	username = strings.TrimSpace(username)
	email = strings.ToLower(strings.TrimSpace(email))
	lg.Info("register.begin")

	exists, err := s.repo.GetByUsername(ctx, username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		lg.Error("register.check_username_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}
	if exists != nil && exists.ID != 0 {
		lg.Info("register.username_exists")
		return nil, apperrors.NewConflictError("用户名已存在")
	}

	exists, err = s.repo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		lg.Error("register.check_email_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}
	if exists != nil && exists.ID != 0 {
		lg.Info("register.email_exists")
		return nil, apperrors.NewConflictError("邮箱已被注册")
	}

	hash, err := s.hashPassword(password)
	if err != nil {
		lg.Error("register.password_hash_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("密码处理失败")
	}

	if avatarFile == nil {
		lg.Error("register.avatar_missing")
		return nil, apperrors.NewParamError("请上传头像")
	}

	avatarKey, avatarURL, err := s.putAvatar(ctx, avatarFile)
	if err != nil {
		lg.Error("register.avatar_post_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("头像存储失败")
	}

	u := &models.User{
		Email:     email,
		Password:  hash,
		Username:  username,
		AvatarURL: avatarURL,
	}

	created, err := s.repo.Create(ctx, u)
	if err != nil {
		lg.Error("register.insert_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("保存失败")
	}

	if s.bus != nil {
		async.PublishWithTimeout(s.bus, lg, "PutAvatar", struct {
			UID       int    `json:"uid"`
			AvatarKey string `json:"avatarKey"`
			AvatarURL string `json:"avatarURL"`
		}{UID: created.ID, AvatarKey: avatarKey, AvatarURL: avatarURL}, 300*time.Millisecond, zap.Int("uid", created.ID))
	}

	lg.Info("register.success", zap.Int("uid", created.ID))
	return &RegisterResult{User: *created}, nil
}

// Logout 把当前 JWT 标记进黑名单。
// 这里不直接删除客户端 token，而是把 jti 写入缓存黑名单，是因为服务端需要在 token 未过期前也能主动失效它。
func (s *UserService) Logout(ctx context.Context, lg *zap.Logger, uid int, claims *utils.Claims) error {
	if uid <= 0 || claims == nil {
		lg.Warn("logout.invalid_input")
		return apperrors.NewUnauthorizedError("未授权")
	}
	lg.Info("logout.begin")

	jti := claims.RegisteredClaims.ID
	exp := claims.RegisteredClaims.ExpiresAt.Time

	err := s.userCache.SetJti(ctx, jti, exp)
	if err != nil {
		lg.Warn("logout.redis_put_error", zap.Error(err))
		return apperrors.NewInternalError("写入缓存出错")
	}
	lg.Info("logout.success")
	return nil
}

type UpdateUserInput struct {
	Email           *string
	Username        *string
	Password        *string
	ConfirmPassword *string
	AvatarFile      *multipart.FileHeader
}

type TokenInfo struct {
	AccessToken    string
	AccessExpireAt time.Time
}

type UpdateUserResult struct {
	User     models.User
	Affected int64
	Token    *TokenInfo
}

// UpdateUser 更新用户资料、密码和头像。
// 密码更新时同步提升 token_version 并返回新 token，是为了让旧 token 立即失效，同时不给前端留下重新登录的额外跳转成本。
func (s *UserService) UpdateUser(ctx context.Context, lg *zap.Logger, uid int, in UpdateUserInput) (*UpdateUserResult, error) {
	lg.Info("update.user.begin")

	update := map[string]interface{}{}

	if in.Username != nil && strings.TrimSpace(*in.Username) != "" {
		username := strings.TrimSpace(*in.Username)
		exists, err := s.repo.GetByUsername(ctx, username)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Error("user.update.check_username_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}
		if exists != nil && exists.ID != 0 && exists.ID != uid {
			return nil, apperrors.NewConflictError("用户名已存在")
		}
		update["username"] = username
	}

	if in.Email != nil && strings.TrimSpace(*in.Email) != "" {
		email := strings.ToLower(strings.TrimSpace(*in.Email))
		exists, err := s.repo.GetByEmail(ctx, email)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Error("user.update.check_email_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}
		if exists != nil && exists.ID != 0 && exists.ID != uid {
			return nil, apperrors.NewConflictError("邮箱已存在")
		}
		update["email"] = email
	}

	oldKey := ""
	newKey := ""
	if in.AvatarFile != nil {
		oldUser, err := s.repo.GetByID(ctx, uid)
		if err != nil {
			lg.Error("user.update.get_old_avatar_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}
		oldKey = utils.NormalizeObjectKey(oldUser.AvatarURL)

		key, newURL, err := s.putAvatar(ctx, in.AvatarFile)
		if err != nil {
			lg.Warn("user.update.avatar_put_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("更新头像失败")
		}
		newKey = key
		update["avatar_url"] = newURL // store full public URL
	}

	if in.Password != nil && in.ConfirmPassword != nil {
		if *in.Password != *in.ConfirmPassword {
			lg.Warn("user.update.password_mismatch")
			return nil, apperrors.NewParamError("两次输入密码不一致")
		}
		hash, err := s.hashPassword(*in.Password)
		if err != nil {
			lg.Error("user.update.password_hash_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("密码处理失败")
		}
		update["password"] = hash
		update["token_version"] = gorm.Expr("token_version + 1")
	} else if in.Password != nil || in.ConfirmPassword != nil {
		lg.Warn("user.update.password_half_provided")
		return nil, apperrors.NewParamError("请同时提供密码与确认密码")
	}

	if len(update) == 0 {
		lg.Info("user.update.no_fields")
		return nil, apperrors.NewParamError("没有需要更新的字段")
	}

	updated, err, affected := s.repo.Update(ctx, uid, update)
	if err != nil {
		if newKey != "" {
			if s.bus != nil {
				async.PublishWithTimeout(s.bus, lg, "DeleteCOS", struct {
					Key string `json:"key"`
				}{Key: newKey}, 300*time.Millisecond) // delete by key still correct
			}
		}
		lg.Error("user.update.db_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("更新失败")
	}

	if newKey != "" && oldKey != "" && oldKey != newKey {
		if s.bus != nil {
			async.PublishWithTimeout(s.bus, lg, "DeleteCOS", struct {
				Key string `json:"key"`
			}{Key: oldKey}, 300*time.Millisecond)
		}
	}

	if affected == 0 {
		lg.Info("user.update.noop")
		return &UpdateUserResult{
			User:     *updated,
			Affected: affected,
			Token:    nil,
		}, nil
	}

	if err := s.userCache.DelProfile(ctx, uid); err != nil {
		lg.Warn("user.update.del_cache_failed", zap.Error(err))
	}

	if _, ok := update["token_version"]; !ok {
		lg.Info("user.update.success", zap.Int64("affected", affected))
		return &UpdateUserResult{
			User:     *updated,
			Affected: affected,
			Token:    nil,
		}, nil
	}

	err = s.userCache.SetVersion(ctx, updated.ID, updated.TokenVersion)
	if err != nil {
		lg.Warn("user.update.putTokenVersion_redis_failed", zap.Error(err))
	}

	tokenStr, exp, _ := utils.GenerateAccessToken(updated.ID, updated.Username, updated.TokenVersion)
	lg.Info("user.update.password_changed", zap.Time("new_access_exp", exp))
	return &UpdateUserResult{
		User:     *updated,
		Affected: affected,
		Token: &TokenInfo{
			AccessToken:    tokenStr,
			AccessExpireAt: exp,
		},
	}, nil
}
