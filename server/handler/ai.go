package handler

import (
	"ToDoList/server/models"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type DraftAIReq struct {
	TaskID      int    `json:"task_id"`
	Title       string `json:"title"`
	Instruction string `json:"instruction"`
	DocType     string `json:"doc_type"`
}

type ContinueAIReq struct {
	TaskID       int    `json:"task_id"`
	Title        string `json:"title"`
	SelectedText string `json:"selected_text"`
	FullContext  string `json:"full_context"`
	Instruction  string `json:"instruction"`
}

type MeetingAIReq struct {
	TaskID      int    `json:"task_id"`
	Title       string `json:"title"`
	Transcript  string `json:"transcript"`
	Notes       string `json:"notes"`
	Instruction string `json:"instruction"`
}

type AIHandler struct {
	svc     *service.AIService
	taskSvc *service.TaskService
}

func NewAIHandler(svc *service.AIService, taskSvc *service.TaskService) *AIHandler {
	return &AIHandler{svc: svc, taskSvc: taskSvc}
}

// DraftPreview
// @Summary Stream AI draft preview
// @Description Streams a Markdown draft preview based on title and instruction. The result is preview-only and does not save content automatically.
// @Tags AI
// @Accept json
// @Produce plain
// @Security BearerAuth
// @Param req body DraftAIReq true "Draft request"
// @Success 200 {string} string "Streamed text"
// @Router /ai/draft/stream [post]
func (h *AIHandler) DraftPreview(c *gin.Context) {
	if !h.ensureReady(c) {
		return
	}
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req DraftAIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	if req.TaskID > 0 {
		task, err := h.loadTaskForAI(c.Request.Context(), lg, uid, req.TaskID)
		if err != nil {
			handleTaskError(c, lg, err, "ai.draft.task_access_failed", start)
			return
		}
		if strings.TrimSpace(req.Title) == "" {
			req.Title = task.Title
		}
		if strings.TrimSpace(req.DocType) == "" {
			req.DocType = task.DocType
		}
	}

	if strings.TrimSpace(req.Title) == "" {
		response.ParamError(c, "title is required")
		return
	}

	h.streamPlainText(c, func(write func(string) error) error {
		return h.svc.StreamDraft(c.Request.Context(), service.AIDraftRequest{
			TaskID:      req.TaskID,
			Title:       req.Title,
			Instruction: req.Instruction,
			DocType:     req.DocType,
		}, write)
	}, lg)
}

// ContinuePreview
// @Summary Stream AI continue or rewrite preview
// @Description Streams preview text for continue or rewrite requests. The result is preview-only and does not save content automatically.
// @Tags AI
// @Accept json
// @Produce plain
// @Security BearerAuth
// @Param req body ContinueAIReq true "Continue request"
// @Success 200 {string} string "Streamed text"
// @Router /ai/continue/stream [post]
func (h *AIHandler) ContinuePreview(c *gin.Context) {
	if !h.ensureReady(c) {
		return
	}
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req ContinueAIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	if req.TaskID > 0 {
		task, err := h.loadTaskForAI(c.Request.Context(), lg, uid, req.TaskID)
		if err != nil {
			handleTaskError(c, lg, err, "ai.continue.task_access_failed", start)
			return
		}
		if strings.TrimSpace(req.Title) == "" {
			req.Title = task.Title
		}
		if strings.TrimSpace(req.FullContext) == "" {
			req.FullContext = task.ContentMD
		}
	}

	h.streamPlainText(c, func(write func(string) error) error {
		return h.svc.StreamContinue(c.Request.Context(), service.AIContinueRequest{
			TaskID:       req.TaskID,
			Title:        req.Title,
			SelectedText: req.SelectedText,
			FullContext:  req.FullContext,
			Instruction:  req.Instruction,
		}, write)
	}, lg)
}

// MeetingPreview
// @Summary Generate AI meeting preview
// @Description Generates a meeting minutes preview, decisions, and action suggestions. The result is preview-only and does not save content or create todos automatically.
// @Tags AI
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param req body MeetingAIReq true "Meeting request"
// @Success 200 {object} response.Resp{data=service.AIMeetingPreview} "Preview generated"
// @Router /ai/meetings/generate [post]
func (h *AIHandler) MeetingPreview(c *gin.Context) {
	if !h.ensureReady(c) {
		return
	}

	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")
	var req MeetingAIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	if req.TaskID > 0 {
		task, err := h.loadTaskForAI(c.Request.Context(), lg, uid, req.TaskID)
		if err != nil {
			handleTaskError(c, lg, err, "ai.meeting.task_access_failed", start)
			return
		}
		if strings.TrimSpace(req.Title) == "" {
			req.Title = task.Title
		}
		if strings.TrimSpace(req.Notes) == "" {
			req.Notes = task.ContentMD
		}
	}

	preview, err := h.svc.GenerateMeetingPreview(c.Request.Context(), service.AIMeetingRequest{
		TaskID:      req.TaskID,
		Title:       req.Title,
		Transcript:  req.Transcript,
		Notes:       req.Notes,
		Instruction: req.Instruction,
	})
	if err != nil {
		handleTaskError(c, lg, err, "ai.meeting.preview_failed", start)
		return
	}

	response.Success(c, preview)
}

func (h *AIHandler) ensureReady(c *gin.Context) bool {
	if h.svc == nil || !h.svc.IsConfigured() {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, errors.New("ai service is not configured"))
		return false
	}
	if _, ok := c.Writer.(http.Flusher); !ok && strings.Contains(c.FullPath(), "/stream") {
		response.ErrorWithStatus(c, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return false
	}
	return true
}

func (h *AIHandler) streamPlainText(c *gin.Context, run func(write func(string) error) error, lg *zap.Logger) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if err := run(func(chunk string) error {
		_, err := c.Writer.WriteString(chunk)
		if err == nil {
			c.Writer.Flush()
		}
		return err
	}); err != nil {
		lg.Error("ai.stream.failed", zap.Error(err))
	}
}

func (h *AIHandler) loadTaskForAI(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*models.Task, error) {
	return h.taskSvc.GetDetail(ctx, lg, uid, taskID)
}
