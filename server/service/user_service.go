package service

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
	bus          async.IEventBus
	sf           singleflight.Group
	hashPassword func(password string) (string, error)
	putAvatar    func(ctx context.Context, file *multipart.FileHeader) (string, string, error)
}

type UserServiceDeps struct {
	Repo         repo.UserRepository
	UserCache    cache.UserCache
	Bus          async.IEventBus
	HashPassword func(password string) (string, error)
	PutAvatar    func(ctx context.Context, file *multipart.FileHeader) (string, string, error)
}

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
		bus:          deps.Bus,
		hashPassword: hashPassword,
		putAvatar:    putAvatar,
	}
}

type LoginResult struct {
	AccessToken    string
	AccessExpireAt time.Time
}

// Login 验证账号密码并签发 JWT
// 业务逻辑：
// 1. 校验用户是否存在
// 2. 比对哈希密码
// 3. 签发 AccessToken (有效期 2h)
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

// GetProfile 获取当前用户信息
// 优先查询 Redis 缓存，如果未命中则查询数据库并回写缓存
// 业务逻辑：
// 1. 查询缓存，处理空值标记（防穿透）
// 2. Singleflight 合并并发请求（防击穿）
// 3. 查询数据库，若不存在则写入空值缓存
// 4. 异步回写缓存（防雪崩：随机TTL在Cache层实现）
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
	val, err, _ := s.sf.Do(key, func() (interface{}, error) {
		// 查询数据库
		dbUser, err := s.repo.GetByID(ctx, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 防穿透：写入空值
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					_ = s.userCache.SetProfile(ctx, uid, nil)
				}()
				return nil, apperrors.NewNotFoundError("用户不存在")
			}
			lg.Error("get_profile.db_failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}

		// 3. 回写缓存 (异步执行)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.userCache.SetProfile(ctx, uid, dbUser); err != nil {
				lg.Warn("get_profile.cache_set_failed", zap.Error(err))
			}
		}()

		return dbUser, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(*models.User), nil
}

type RegisterResult struct {
	User models.User
}

// Register 用户注册
// 业务逻辑：
// 1. 校验用户名、邮箱是否已存在
// 2. 校验密码长度
// 3. 校验头像文件
// 4. 哈希密码
// 5. 保存用户到数据库
// 6. 发布注册事件（异步处理）
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

// Logout 用户注销, 采用 JWT 黑名单机制
// 业务逻辑：
// 1. 校验用户ID和JWT Claims
// 2. 将JWT ID加入黑名单（设置过期时间为过期时间）
// 3. 异步删除缓存中的用户信息
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

// UpdateUser 更新用户信息
// 业务逻辑：
// 1. 校验用户ID和输入参数
// 2. 校验用户名、邮箱是否已存在（不包括当前用户）
// 3. 校验密码长度
// 4. 校验头像文件
// 5. 哈希密码
// 6. 更新用户信息到数据库
// 7. 发布更新事件（异步处理）
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
