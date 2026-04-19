package handler

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CreateProjectReq struct {
	Name  string `json:"name" binding:"required,min=1,max=128"`
	Color string `json:"color"`
}

type UpdateProjectReq struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	SortOrder *int64  `json:"sort_order"`
}

type ProjectHandler struct {
	svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

// Create
// @Summary 创建项目
// @Description 创建新的项目
// @Tags Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param req body CreateProjectReq true "项目信息"
// @Success 200 {object} response.Resp{data=models.Project} "创建成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 409 {object} response.Resp "项目已存在"
// @Router /projects [post]
func (h *ProjectHandler) Create(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req CreateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "项目名称不能为空")
		return
	}

	project, err := h.svc.Create(c.Request.Context(), lg, uid, req.Name, req.Color)
	if err != nil {
		handleProjectError(c, lg, err, "project.create.failed", start)
		return
	}

	response.Success(c, project)
}

// GetProjectByID
// @Summary 获取项目详情
// @Description 根据ID获取项目信息
// @Tags Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Success 200 {object} response.Resp{data=models.Project} "获取成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "项目不存在"
// @Router /projects/{id} [get]
func (h *ProjectHandler) GetProjectByID(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "项目ID无效")
		return
	}

	project, err := h.svc.GetByID(c.Request.Context(), lg, uid, id)
	if err != nil {
		handleProjectError(c, lg, err, "project.get.failed", start)
		return
	}

	response.Success(c, project)
}

// Search
// @Summary 项目列表/搜索
// @Description 获取项目列表，支持按名称搜索
// @Tags Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param name query string false "项目名称"
// @Param page query int false "页码"
// @Param size query int false "每页数量"
// @Success 200 {object} response.Resp{data=response.PageResult} "获取成功"
// @Router /projects [get]
func (h *ProjectHandler) Search(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	name := c.Query("name")

	result, err := h.svc.List(c.Request.Context(), lg, uid, service.ProjectListInput{
		Page: page,
		Size: size,
		Name: name,
	})
	if err != nil {
		handleProjectError(c, lg, err, "project.list.failed", start)
		return
	}

	response.PageData(c, result.Projects, result.Total, page, size)
}

// Update
// @Summary 更新项目
// @Description 更新项目信息（名称、颜色等）
// @Tags Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Param req body UpdateProjectReq true "更新信息"
// @Success 200 {object} response.Resp "更新成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "项目不存在"
// @Router /projects/{id} [patch]
func (h *ProjectHandler) Update(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "项目ID无效")
		return
	}

	var req UpdateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	_, err, _ = h.svc.Update(c.Request.Context(), lg, uid, id, service.UpdateProjectInput{
		Name:      req.Name,
		Color:     req.Color,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		handleProjectError(c, lg, err, "project.update.failed", start)
		return
	}

	response.SuccessWithMsg(c, "更新成功", nil)
}

// Delete
// @Summary 删除项目
// @Description 删除指定项目及其关联任务
// @Tags Project
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Success 200 {object} response.Resp "删除成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "项目不存在"
// @Router /projects/{id} [delete]
func (h *ProjectHandler) Delete(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "项目ID无效")
		return
	}

	_, err = h.svc.Delete(c.Request.Context(), lg, uid, id)
	if err != nil {
		handleProjectError(c, lg, err, "project.delete.failed", start)
		return
	}

	response.SuccessWithMsg(c, "删除成功", nil)
}

func handleProjectError(c *gin.Context, lg *zap.Logger, err error, logMsg string, start time.Time) {
	var appErr *apperrors.Error
	if apperrors.As(err, &appErr) {
		lg.Warn(logMsg, zap.Int("code", int(appErr.Code)), zap.Duration("elapsed_ms", time.Since(start)))
		switch appErr.Code {
		case apperrors.CodeParamInvalid:
			response.ParamError(c, appErr.Message)
		case apperrors.CodeNotFound:
			response.NotFound(c, appErr.Message)
		case apperrors.CodeConflict:
			response.Conflict(c, appErr.Message)
		case apperrors.CodeForbidden:
			response.Forbidden(c, appErr.Message)
		default:
			response.Error(c, err)
		}
		return
	}
	lg.Error(logMsg, zap.Error(err), zap.Duration("elapsed_ms", time.Since(start)))
	response.Error(c, err)
}
