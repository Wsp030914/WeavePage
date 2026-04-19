package handler

import (
	"ToDoList/server/config"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CreateTaskReq struct {
	Title     string     `json:"title" binding:"required,min=1,max=200"`
	ProjectID int        `json:"project_id" binding:"required"`
	ContentMD *string    `json:"content_md"`
	Priority  *int       `json:"priority"`
	Status    *string    `json:"status"`
	DueAt     *time.Time `json:"due_at"`
}

type UpdateTaskReq struct {
	Title           *string    `json:"title"`
	ProjectID       *int       `json:"project_id"`
	ContentMD       *string    `json:"content_md"`
	Priority        *int       `json:"priority"`
	Status          *string    `json:"status"`
	SortOrder       *int64     `json:"sort_order"`
	ReDueAt         *time.Time `json:"due_at"`
	ClearDue        *bool      `json:"clear_due_at"`
	ExpectedVersion *int       `json:"expected_version"`
}

type DueCallbackReq struct {
	TaskID      int        `json:"task_id" binding:"required,min=1"`
	TriggeredAt *time.Time `json:"triggered_at"`
}

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

type AddMemberReq struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required,oneof=editor viewer"`
}

type RemoveMemberReq struct {
	UserID int `json:"user_id" binding:"required"`
}

// AddMember
// @Summary 添加任务成员
// @Description 邀请用户参与任务
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Param task_id path int true "任务ID"
// @Param req body AddMemberReq true "成员信息"
// @Success 200 {object} response.Resp "添加成功"
// @Router /projects/{id}/tasks/{task_id}/members [post]
func (h *TaskHandler) AddMember(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("task_id"))
	if err != nil {
		response.ParamError(c, "任务ID无效")
		return
	}

	var req AddMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	if err := h.svc.AddMember(c.Request.Context(), lg, uid, taskID, req.Email, req.Role); err != nil {
		handleTaskError(c, lg, err, "task.add_member.failed", start)
		return
	}

	response.Success(c, nil)
}

// RemoveMember
// @Summary 移除任务成员
// @Description 移除任务成员
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Param task_id path int true "任务ID"
// @Param req body RemoveMemberReq true "成员ID"
// @Success 200 {object} response.Resp "移除成功"
// @Router /projects/{id}/tasks/{task_id}/members [delete]
func (h *TaskHandler) RemoveMember(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("task_id"))
	if err != nil {
		response.ParamError(c, "任务ID无效")
		return
	}

	var req RemoveMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	if err := h.svc.RemoveMember(c.Request.Context(), lg, uid, taskID, req.UserID); err != nil {
		handleTaskError(c, lg, err, "task.remove_member.failed", start)
		return
	}

	response.Success(c, nil)
}

// ListMyTasks
// @Summary 查看我参与的任务
// @Description 获取当前用户参与的所有任务
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param size query int false "每页数量"
// @Success 200 {object} response.Resp{data=service.TaskListResult} "获取成功"
// @Router /tasks/me [get]
func (h *TaskHandler) ListMyTasks(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	status := strings.TrimSpace(c.Query("status"))

	var dueStart *time.Time
	if raw := strings.TrimSpace(c.Query("due_start")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.ParamError(c, "due_start must be RFC3339")
			return
		}
		localParsed := parsed.In(time.Local)
		dueStart = &localParsed
	}

	var dueEnd *time.Time
	if raw := strings.TrimSpace(c.Query("due_end")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.ParamError(c, "due_end must be RFC3339")
			return
		}
		localParsed := parsed.In(time.Local)
		dueEnd = &localParsed
	}

	res, err := h.svc.ListMyTasks(c.Request.Context(), lg, uid, service.MyTaskListInput{
		Page:     page,
		Size:     size,
		Status:   status,
		DueStart: dueStart,
		DueEnd:   dueEnd,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.list_my_tasks.failed", start)
		return
	}

	response.Success(c, res)
}

// Create
// @Summary 创建任务
// @Description 在指定项目中创建新任务
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param req body CreateTaskReq true "任务信息"
// @Success 200 {object} response.Resp{data=models.Task} "创建成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 403 {object} response.Resp "权限不足"
// @Router /tasks [post]
func (h *TaskHandler) Create(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req CreateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	task, err := h.svc.Create(c.Request.Context(), lg, uid, service.CreateTaskInput{
		Title:     req.Title,
		ProjectID: req.ProjectID,
		ContentMD: req.ContentMD,
		Priority:  req.Priority,
		Status:    req.Status,
		DueAt:     req.DueAt,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.create.failed", start)
		return
	}

	response.Success(c, task)
}

// Update
// @Summary 更新任务
// @Description 更新任务详情（标题、内容、状态、优先级等）
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "项目ID"
// @Param task_id path int true "任务ID"
// @Param req body UpdateTaskReq true "更新信息"
// @Success 200 {object} response.Resp "更新成功"
// @Failure 400 {object} response.Resp "参数错误"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "任务不存在"
// @Router /projects/{id}/tasks/{task_id} [patch]
func (h *TaskHandler) Update(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	pid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "项目ID无效")
		return
	}

	taskID, err := strconv.Atoi(c.Param("task_id"))
	if err != nil {
		response.ParamError(c, "任务ID无效")
		return
	}

	var req UpdateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	_, err, _ = h.svc.Update(c.Request.Context(), lg, uid, pid, taskID, service.UpdateTaskInput{
		Title:           req.Title,
		ProjectID:       req.ProjectID,
		ContentMD:       req.ContentMD,
		Priority:        req.Priority,
		Status:          req.Status,
		SortOrder:       req.SortOrder,
		ReDueAt:         req.ReDueAt,
		ClearDue:        req.ClearDue,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.update.failed", start)
		return
	}

	response.SuccessWithMsg(c, "更新成功", nil)
}

// Delete
// @Summary 删除任务
// @Description 删除指定任务
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "任务ID"
// @Success 200 {object} response.Resp "删除成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "任务不存在"
// @Router /tasks/{id} [delete]
func (h *TaskHandler) Delete(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "任务ID无效")
		return
	}

	_, err = h.svc.Delete(c.Request.Context(), lg, uid, id)
	if err != nil {
		handleTaskError(c, lg, err, "task.delete.failed", start)
		return
	}

	response.SuccessWithMsg(c, "删除成功", nil)
}

// GetDetail
// @Summary 查询任务详情
// @Description 获取单个任务的详细信息
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "任务ID"
// @Success 200 {object} response.Resp{data=models.Task} "查询成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 404 {object} response.Resp "任务不存在"
// @Router /tasks/{id} [get]
func (h *TaskHandler) GetDetail(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "任务ID无效")
		return
	}

	task, err := h.svc.GetDetail(c.Request.Context(), lg, uid, taskID)
	if err != nil {
		handleTaskError(c, lg, err, "task.get_detail.failed", start)
		return
	}

	response.Success(c, task)
}

// List
// @Summary 任务列表
// @Description 获取指定项目的任务列表
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param project_id query int true "项目ID"
// @Param status query string false "任务状态 (todo/done)"
// @Param page query int false "页码"
// @Param size query int false "每页数量"
// @Success 200 {object} response.Resp{data=response.PageResult} "获取成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Router /tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	status := c.Query("status")
	pid, err := strconv.Atoi(c.Query("project_id"))
	if err != nil {
		response.ParamError(c, "项目ID无效")
		return
	}

	result, err := h.svc.List(c.Request.Context(), lg, uid, service.TaskListInput{
		Page:   page,
		Size:   size,
		Status: status,
		Pid:    pid,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.list.failed", start)
		return
	}

	response.PageData(c, result.Tasks, result.Total, page, size)
}

// DueCallback
// @Summary 任务到期回调
// @Description 内部调度器回调接口，用于触发任务到期通知
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-Scheduler-Token header string true "调度令牌"
// @Param req body DueCallbackReq true "回调信息"
// @Success 200 {object} response.Resp "回调处理成功"
// @Failure 401 {object} response.Resp "令牌无效"
// @Router /internal/due_callback [post]
func (h *TaskHandler) DueCallback(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()

	token := strings.TrimSpace(c.GetHeader(service.SchedulerTokenHeader))
	expectedToken := ""
	if config.GlobalConfig != nil {
		expectedToken = strings.TrimSpace(config.GlobalConfig.DueScheduler.CallbackToken)
	}
	if token == "" || expectedToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		response.Unauthorized(c, "无效的调度令牌")
		return
	}

	var req DueCallbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "无效的回调负载")
		return
	}

	notified, err := h.svc.HandleDueCallback(c.Request.Context(), lg, service.DueCallbackInput{
		TaskID:      req.TaskID,
		TriggeredAt: req.TriggeredAt,
	})
	if err != nil {
		lg.Error("task.due_callback.failed", zap.Error(err), zap.Duration("elapsed_ms", time.Since(start)))
		var appErr *apperrors.Error
		if apperrors.As(err, &appErr) && appErr.Code == apperrors.CodeParamInvalid {
			response.ErrorWithStatus(c, http.StatusBadRequest, err)
			return
		}
		response.ErrorWithStatus(c, http.StatusInternalServerError, err)
		return
	}

	response.SuccessWithMsg(c, "ok", gin.H{"notified": notified})
}

func handleTaskError(c *gin.Context, lg *zap.Logger, err error, logMsg string, start time.Time) {
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
