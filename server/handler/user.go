package handler

// 文件说明：这个文件负责用户账号相关的 HTTP 接口。
// 实现方式：接口层处理 JSON 或 multipart 参数绑定、上下文提取和统一错误响应，账号规则放在 UserService。
// 这样做的好处是登录、注册、资料更新这些协议差异都留在边界层，核心账号逻辑仍然保持集中。

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"errors"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"go.uber.org/zap"
)

type RegisterReq struct {
	Email           string `form:"email" binding:"required,email,max=255"`
	Username        string `form:"username" binding:"required,min=2,max=64"`
	Password        string `form:"password" binding:"required,min=8,max=72"`
	ConfirmPassword string `form:"confirm_password" binding:"required,eqfield=Password"`
}

type LoginReq struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type UpdateUserReq struct {
	Email           *string `form:"email" binding:"omitempty,email,max=255"`
	Username        *string `form:"username" binding:"omitempty,min=2,max=64"`
	Password        *string `form:"password" binding:"omitempty,min=8,max=72,required_with=ConfirmPassword"`
	ConfirmPassword *string `form:"confirm_password" binding:"omitempty,required_with=Password,eqfield=Password"`
}

type UserHandler struct {
	svc *service.UserService
}

// NewUserHandler 创建用户接口处理器。
func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Login 处理用户登录请求。
// @Summary 用户登录
// @Description 使用用户名或邮箱和密码登录
// @Tags User
// @Accept json
// @Produce json
// @Param req body LoginReq true "登录信息"
// @Success 200 {object} response.Resp{data=map[string]interface{}} "登录成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 401 {object} response.Resp "用户名或密码错误"
// @Router /login [post]
func (u *UserHandler) Login(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		lg.Warn("user.login.param_bind_failed", zap.Error(err))
		response.ParamError(c, "参数格式有误")
		return
	}

	res, err := u.svc.Login(c.Request.Context(), lg, req.Username, req.Password)
	if err != nil {
		handleError(c, lg, err, "user.login.failed", start)
		return
	}

	lg.Info("user.login.success", zap.Duration("elapsed_ms", time.Since(start)))
	response.Success(c, gin.H{
		"access_token":      res.AccessToken,
		"token_type":        "Bearer",
		"access_expires_at": res.AccessExpireAt.UTC().Format(time.RFC3339),
	})
}

// Register 处理用户注册请求。
// @Summary 用户注册
// @Description 注册新用户，支持头像上传
// @Tags User
// @Accept multipart/form-data
// @Produce json
// @Param email formData string true "邮箱"
// @Param username formData string true "用户名"
// @Param password formData string true "密码"
// @Param confirm_password formData string true "确认密码"
// @Param file formData file true "头像文件"
// @Success 200 {object} response.Resp{data=models.User} "注册成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 409 {object} response.Resp "用户名或邮箱已存在"
// @Router /register [post]
func (u *UserHandler) Register(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()

	var req RegisterReq
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		lg.Warn("user.register.param_bind_failed", zap.Error(err))
		response.ParamError(c, "参数格式错误")
		return
	}
	lg = lg.With(zap.String("username", req.Username), zap.String("email", req.Email))
	fh, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			lg.Warn("user.register.avatar_missing", zap.Error(err))
			response.ParamError(c, "请上传头像")
			return
		}
		lg.Warn("user.register.avatar_read_failed", zap.Error(err))
		response.ParamError(c, "头像上传失败")
		return
	}
	res, err := u.svc.Register(c.Request.Context(), lg, req.Email, req.Username, req.Password, fh)
	if err != nil {
		handleError(c, lg, err, "user.register.failed", start)
		return
	}

	lg.Info("user.register.success", zap.Int("uid", res.User.ID), zap.Duration("elapsed_ms", time.Since(start)))
	response.Success(c, res.User)
}

// Logout 注销当前登录态。
// @Summary 退出登录
// @Description 注销当前用户的登录状态
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Resp "退出成功"
// @Failure 401 {object} response.Resp "未授权"
// @Router /logout [post]
func (u *UserHandler) Logout(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()

	uid := c.GetInt("uid")
	if uid <= 0 {
		response.Unauthorized(c, "未授权")
		return
	}
	lg = lg.With(zap.Int("uid", uid))

	v, ok := c.Get("claims")
	if !ok {
		response.Unauthorized(c, "用户未授权")
		return
	}
	claims, ok := v.(*utils.Claims)
	if !ok {
		response.Unauthorized(c, "用户未授权")
		return
	}

	if err := u.svc.Logout(c.Request.Context(), lg, uid, claims); err != nil {
		handleError(c, lg, err, "user.logout.failed", start)
		return
	}

	lg.Info("user.logout.success", zap.Duration("elapsed_ms", time.Since(start)))
	response.SuccessWithMsg(c, "已退出登录", nil)
}

// GetProfile 返回当前登录用户的资料。
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的详细信息
// @Tags User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Resp{data=models.User} "获取成功"
// @Failure 401 {object} response.Resp "未授权"
// @Router /users/me [get]
func (u *UserHandler) GetProfile(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")
	lg = lg.With(zap.Int("uid", uid))

	user, err := u.svc.GetProfile(c.Request.Context(), lg, uid)
	if err != nil {
		handleError(c, lg, err, "user.get_profile.failed", start)
		return
	}

	lg.Info("user.get_profile.success", zap.Duration("elapsed_ms", time.Since(start)))
	response.Success(c, user)
}

// Update 更新当前用户资料。
// @Summary 更新用户信息
// @Description 更新当前用户的个人资料、头像和密码
// @Tags User
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param email formData string false "邮箱"
// @Param username formData string false "用户名"
// @Param password formData string false "密码"
// @Param confirm_password formData string false "确认密码"
// @Param file formData file false "头像文件"
// @Success 200 {object} response.Resp{data=models.User} "更新成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 409 {object} response.Resp "用户名或邮箱已存在"
// @Router /users/me [patch]
func (u *UserHandler) Update(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")
	lg = lg.With(zap.Int("uid", uid))

	var req UpdateUserReq
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		lg.Warn("user.update.param_bind_failed", zap.Error(err))
		response.ParamError(c, "参数格式有误")
		return
	}

	var fh *multipart.FileHeader
	file, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			fh = nil
		} else {
			lg.Warn("user.update.avatar_read_failed", zap.Error(err))
			response.ParamError(c, "头像上传失败")
			return
		}
	} else {
		fh = file
	}

	in := service.UpdateUserInput{
		Email:           req.Email,
		Username:        req.Username,
		Password:        req.Password,
		ConfirmPassword: req.ConfirmPassword,
		AvatarFile:      fh,
	}

	res, err := u.svc.UpdateUser(c.Request.Context(), lg, uid, in)
	if err != nil {
		handleError(c, lg, err, "user.update.failed", start)
		return
	}

	if res.Token == nil {
		lg.Info("user.update.success", zap.Duration("elapsed_ms", time.Since(start)))
		response.Success(c, res.User)
		return
	}

	lg.Info("user.update.success_with_token", zap.Duration("elapsed_ms", time.Since(start)))
	response.Success(c, gin.H{
		"access_token":      res.Token.AccessToken,
		"token_type":        "Bearer",
		"access_expires_at": res.Token.AccessExpireAt.UTC().Format(time.RFC3339),
		"user":              res.User,
	})
}

// handleError 统一把用户域应用错误映射成 HTTP 响应。
func handleError(c *gin.Context, lg *zap.Logger, err error, logMsg string, start time.Time) {
	var appErr *apperrors.Error
	if apperrors.As(err, &appErr) {
		lg.Warn(logMsg, zap.Int("code", int(appErr.Code)), zap.Duration("elapsed_ms", time.Since(start)))
		switch appErr.Code {
		case apperrors.CodeParamInvalid:
			response.ParamError(c, appErr.Message)
		case apperrors.CodeUnauthorized:
			response.Unauthorized(c, appErr.Message)
		case apperrors.CodeNotFound:
			response.NotFound(c, appErr.Message)
		case apperrors.CodeConflict:
			response.Conflict(c, appErr.Message)
		default:
			response.Error(c, err)
		}
		return
	}
	lg.Error(logMsg, zap.Error(err), zap.Duration("elapsed_ms", time.Since(start)))
	response.Error(c, err)
}
