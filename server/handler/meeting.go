package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateMeetingReq struct {
	ProjectID *int   `json:"project_id"`
	Title     string `json:"title"`
}

type CreateMeetingActionTodoReq struct {
	Title string     `json:"title" binding:"required"`
	DueAt *time.Time `json:"due_at"`
}

type MeetingHandler struct {
	svc *service.TaskService
}

func NewMeetingHandler(svc *service.TaskService) *MeetingHandler {
	return &MeetingHandler{svc: svc}
}

// Create
// @Summary 新建会议纪要
// @Description 新建 Notion 风格会议纪要，默认启用协作并写入会议模板。
// @Tags Meeting
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param req body CreateMeetingReq false "可选：指定 project_id 或自定义标题"
// @Success 200 {object} response.Resp{data=service.MeetingCreateResult} "创建成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 500 {object} response.Resp "系统错误"
// @Router /meetings [post]
func (h *MeetingHandler) Create(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req CreateMeetingReq
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ParamError(c, "参数格式有误")
			return
		}
	}

	result, err := h.svc.CreateMeetingNote(c.Request.Context(), lg, uid, service.CreateMeetingInput{
		ProjectID: req.ProjectID,
		Title:     req.Title,
		Now:       time.Now(),
	})
	if err != nil {
		handleTaskError(c, lg, err, "meeting.create.failed", start)
		return
	}

	response.Success(c, result)
}

// CreateActionTodo creates a lightweight todo from a meeting action item.
// @Summary Create todo from meeting action
// @Description Creates a lightweight todo in the same space as the meeting note.
// @Tags Meeting
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Meeting document ID"
// @Param req body CreateMeetingActionTodoReq true "Action item"
// @Success 200 {object} response.Resp{data=models.Task} "Created"
// @Router /meetings/{id}/actions [post]
func (h *MeetingHandler) CreateActionTodo(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")
	meetingID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid meeting id")
		return
	}

	var req CreateMeetingActionTodoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	task, err := h.svc.CreateTodoFromMeetingAction(c.Request.Context(), lg, uid, meetingID, service.MeetingActionTodoInput{
		Title: req.Title,
		DueAt: req.DueAt,
	})
	if err != nil {
		handleTaskError(c, lg, err, "meeting.action_todo.failed", start)
		return
	}
	response.Success(c, task)
}
