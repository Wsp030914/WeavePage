package handler

// 文件说明：这个文件负责任务、文档、回收站和到期回调相关的 HTTP 接口。
// 实现方式：handler 只做参数解析、鉴权上下文提取、错误映射和响应组装，业务规则放在 service 层。
// 这样做的好处是接口层保持轻薄，后续无论扩 Swagger、调整协议还是接 WebSocket 补偿，都不需要把业务逻辑散落到控制器里。
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
	Title             string     `json:"title" binding:"required,min=1,max=200"`
	ProjectID         int        `json:"project_id" binding:"required"`
	ContentMD         *string    `json:"content_md"`
	DocType           string     `json:"doc_type"`
	CollaborationMode string     `json:"collaboration_mode"`
	Priority          *int       `json:"priority"`
	Status            *string    `json:"status"`
	DueAt             *time.Time `json:"due_at"`
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

type SaveDocumentContentReq struct {
	ContentMD       *string `json:"content_md" binding:"required"`
	ExpectedVersion *int    `json:"expected_version" binding:"required"`
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

// AddMember 为任务添加协作成员。
// 这里要求前端传邮箱而不是 userID，是为了兼容邀请式协作入口，避免调用方先额外查一次用户列表。
// @Summary 添加任务成员
// @Description 邀请一个用户加入任务协作，并授予 editor/viewer 角色
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

// RemoveMember 从任务中移除协作成员。
// 这个接口保留显式的移除动作，而不是和更新角色复用一个 patch，是为了让审计语义更清晰。
// @Summary 移除任务成员
// @Description 从任务协作成员列表里移除指定用户
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

// ListMyTasks 返回当前用户参与的任务视图。
// 这里直接支持状态和时间窗筛选，是为了让“我的任务”“未来七天”“日历”等前端视图复用同一条后端查询链路。
// @Summary 查看我参与的任务
// @Description 获取当前用户参与的任务，并支持状态和截止时间过滤
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

// Create 创建任务或文档。
// 文档、会议、轻量待办当前都复用 task 聚合根，因此接口层统一从这里进入，再由 service 根据 doc_type 等字段分流规则。
// @Summary 创建任务
// @Description 在指定空间中创建任务或文档
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
		Title:             req.Title,
		ProjectID:         req.ProjectID,
		ContentMD:         req.ContentMD,
		DocType:           req.DocType,
		CollaborationMode: req.CollaborationMode,
		Priority:          req.Priority,
		Status:            req.Status,
		DueAt:             req.DueAt,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.create.failed", start)
		return
	}

	response.Success(c, task)
}

// Update 更新任务元数据。
// 这里保留 expected_version 入口，是为了让前端可以把 HTTP 写路径和实时协同里的 CAS 语义对齐，避免静默覆盖。
// @Summary 更新任务
// @Description 更新任务的标题、状态、优先级、截止时间和排序等元数据
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

// SaveDocumentContent
// @Summary Save plain Markdown document content
// @Description Saves plain Markdown content for diary documents. Only the owner can save, and this does not write the Yjs update log.
// @Tags Document
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Document ID"
// @Param req body SaveDocumentContentReq true "Content payload"
// @Success 200 {object} response.Resp{data=models.Task} "Saved"
// @Failure 400 {object} response.Resp "Invalid parameters"
// @Failure 403 {object} response.Resp "Forbidden"
// @Failure 404 {object} response.Resp "Document not found"
// @Router /documents/{id}/content [patch]
func (h *TaskHandler) SaveDocumentContent(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid document id")
		return
	}

	var req SaveDocumentContentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	task, err, _ := h.svc.SavePlainDocumentContent(c.Request.Context(), lg, uid, taskID, service.SavePlainDocumentContentInput{
		ContentMD:       *req.ContentMD,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		handleTaskError(c, lg, err, "document.content.save.failed", start)
		return
	}

	response.Success(c, task)
}

// ListTrash
// @Summary List trashed tasks/documents
// @Description Returns the current user's soft-deleted tasks/documents.
// @Tags Trash
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page"
// @Param size query int false "Page size"
// @Success 200 {object} response.Resp{data=response.PageResult} "Loaded"
// @Router /trash/tasks [get]
func (h *TaskHandler) ListTrash(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	result, err := h.svc.ListTrash(c.Request.Context(), lg, uid, service.TrashListInput{
		Page: page,
		Size: size,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.list_trash.failed", start)
		return
	}

	response.PageData(c, result.Tasks, result.Total, page, size)
}

// RestoreFromTrash
// @Summary Restore task/document from trash
// @Description Restores a soft-deleted task/document back to its space.
// @Tags Trash
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Task ID"
// @Success 200 {object} response.Resp{data=service.TrashRestoreResult} "Restored"
// @Router /trash/tasks/{id}/restore [post]
func (h *TaskHandler) RestoreFromTrash(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid task id")
		return
	}

	result, err := h.svc.RestoreFromTrash(c.Request.Context(), lg, uid, id)
	if err != nil {
		handleTaskError(c, lg, err, "task.restore.failed", start)
		return
	}

	response.Success(c, result)
}

// DeleteFromTrash
// @Summary Permanently delete task/document from trash
// @Description Hard-deletes a task/document that is already in trash.
// @Tags Trash
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Task ID"
// @Success 200 {object} response.Resp "Deleted"
// @Router /trash/tasks/{id} [delete]
func (h *TaskHandler) DeleteFromTrash(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid task id")
		return
	}

	_, err = h.svc.DeleteFromTrash(c.Request.Context(), lg, uid, id)
	if err != nil {
		handleTaskError(c, lg, err, "task.delete_from_trash.failed", start)
		return
	}

	response.SuccessWithMsg(c, "deleted", nil)
}

// Delete 把任务移入回收站。
// 当前默认走软删除而不是直接物理删除，是为了给文档型任务保留可恢复入口，同时兼容前端的实时删除事件。
// @Summary 删除任务
// @Description 把指定任务移入回收站
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

// GetDetail 返回单个任务详情。
// 详情接口仍然独立存在，是为了给侧边栏、详情页直达、回收站恢复后回跳等场景提供稳定读取入口。
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

// List 返回指定空间下的任务列表。
// 这里先走缓存链路，再由 service 层决定是否回退数据库，因此 handler 只关心分页参数，不感知缓存细节。
// @Summary 任务列表
// @Description 获取指定空间下的任务列表
// @Tags Task
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param project_id query int true "项目ID"
// @Param status query string false "任务状态(todo/done)"
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

// DueCallback 处理调度器回调的到期提醒。
// 这里单独校验调度 token，是为了把内部回调面和普通用户接口隔离开，避免误调用公开 API 触发通知。
// @Summary 任务到期回调
// @Description 调度器内部回调接口，用于触发任务到期通知
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-Scheduler-Token header string true "调度令牌"
// @Param req body DueCallbackReq true "回调信息"
// @Success 200 {object} response.Resp "回调处理成功"
// @Failure 401 {object} response.Resp "令牌无效"
// @Router /api/internal/scheduler/task-due [post]
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

// handleTaskError 统一把 service 返回的应用错误映射成 HTTP 响应。
// 这样做的好处是任务域相关接口可以复用同一套错误翻译规则，减少每个 handler 重复写 switch。
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
