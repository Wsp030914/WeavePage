package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"time"

	"github.com/gin-gonic/gin"
)

// CreateMeetingReq describes the optional meeting note creation payload.
type CreateMeetingReq struct {
	ProjectID *int   `json:"project_id"`
	Title     string `json:"title"`
}

// MeetingHandler handles meeting-note semantic endpoints.
type MeetingHandler struct {
	svc *service.TaskService
}

// NewMeetingHandler creates a meeting handler.
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
