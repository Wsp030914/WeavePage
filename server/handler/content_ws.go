package handler

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/realtime"
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ContentWSHandler struct {
	taskSvc *service.TaskService
	authSvc *service.AuthService
	hub     *realtime.ContentHub
}

func NewContentWSHandler(taskSvc *service.TaskService, authSvc *service.AuthService, hub *realtime.ContentHub) *ContentWSHandler {
	return &ContentWSHandler{
		taskSvc: taskSvc,
		authSvc: authSvc,
		hub:     hub,
	}
}

// TaskContent
// @Summary Task content collaboration WebSocket
// @Description Upgrades to a task content WebSocket stream for CONTENT_INIT, CONTENT_SYNC, CONTENT_UPDATE, CONTENT_ACK, CONTENT_ERROR, PING, and PONG messages. Authenticate with Authorization: Bearer <token> or the token query parameter. Diary documents are rejected; save them with PATCH /api/v1/documents/{id}/content.
// @Tags Realtime
// @Accept json
// @Produce json
// @Param id path int true "Task ID"
// @Param Authorization header string false "Bearer JWT"
// @Param token query string false "JWT when a WebSocket client cannot send Authorization"
// @Param last_update_id query int false "Last seen task_content_updates id"
// @Success 101 {object} realtime.ContentServerMessage "Switching Protocols; subsequent frames use this message schema"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 401 {object} response.Resp "Missing or invalid token"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Task not found"
// @Failure 503 {object} response.Resp "WebSocket hub is not configured"
// @Router /tasks/{id}/content/ws [get]
func (h *ContentWSHandler) TaskContent(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	if h.hub == nil {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, apperrors.NewInternalError("task content websocket is not configured"))
		return
	}

	claims, err := authenticateWebSocket(c, h.authSvc)
	if err != nil {
		response.Unauthorized(c, "token已不可用")
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil || taskID <= 0 {
		response.ParamError(c, "任务ID无效")
		return
	}

	session, err := h.taskSvc.OpenTaskContentSession(c.Request.Context(), lg, claims.UID, taskID)
	if err != nil {
		handleTaskError(c, lg, err, "task.content.open_session_failed", start)
		return
	}

	cursor, _ := strconv.ParseInt(strings.TrimSpace(c.Query("last_update_id")), 10, 64)
	if cursor < 0 {
		cursor = 0
	}

	conn, err := realtime.UpgradeContent(c.Writer, c.Request)
	if err != nil {
		lg.Warn("task.content.ws_upgrade_failed", zap.Int("task_id", taskID), zap.Error(err))
		return
	}

	h.hub.HandleConnection(c.Request.Context(), conn, *session, cursor, lg.With(zap.Int("task_id", taskID), zap.Int("uid", claims.UID)))
}
