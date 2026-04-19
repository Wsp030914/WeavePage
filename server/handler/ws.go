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

type ProjectWSHandler struct {
	taskSvc *service.TaskService
	authSvc *service.AuthService
	hub     *realtime.ProjectHub
}

func NewProjectWSHandler(taskSvc *service.TaskService, authSvc *service.AuthService, hub *realtime.ProjectHub) *ProjectWSHandler {
	return &ProjectWSHandler{
		taskSvc: taskSvc,
		authSvc: authSvc,
		hub:     hub,
	}
}

// ProjectEvents
// @Summary Project realtime WebSocket
// @Description Upgrades to a project WebSocket stream for PROJECT_INIT, PROJECT_SYNC, TASK_CREATED, TASK_UPDATED, TASK_DELETED, PRESENCE_SNAPSHOT, TASK_LOCKED, TASK_UNLOCKED, LOCK_ERROR, PING, and PONG messages. Authenticate with Authorization: Bearer <token> or the token query parameter.
// @Tags Realtime
// @Accept json
// @Produce json
// @Param id path int true "Project ID"
// @Param Authorization header string false "Bearer JWT"
// @Param token query string false "JWT when a WebSocket client cannot send Authorization"
// @Param cursor query int false "Last seen task event id"
// @Param last_event_id query int false "Alias of cursor"
// @Success 101 {object} realtime.ProjectServerMessage "Switching Protocols; subsequent frames use this message schema"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 401 {object} response.Resp "Missing or invalid token"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Project not found"
// @Failure 503 {object} response.Resp "WebSocket hub is not configured"
// @Router /projects/{id}/ws [get]
func (h *ProjectWSHandler) ProjectEvents(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	if h.hub == nil {
		response.ErrorWithStatus(c, http.StatusServiceUnavailable, apperrors.NewInternalError("project websocket is not configured"))
		return
	}

	claims, err := authenticateWebSocket(c, h.authSvc)
	if err != nil {
		response.Unauthorized(c, "token已不可用")
		return
	}

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil || projectID <= 0 {
		response.ParamError(c, "项目ID无效")
		return
	}

	session, err := h.taskSvc.OpenProjectRealtimeSession(c.Request.Context(), lg, claims.UID, projectID)
	if err != nil {
		handleTaskError(c, lg, err, "project.ws.open_session_failed", start)
		return
	}
	session.Username = claims.Username

	cursor, _ := strconv.ParseInt(strings.TrimSpace(c.DefaultQuery("cursor", c.Query("last_event_id"))), 10, 64)
	if cursor < 0 {
		cursor = 0
	}

	conn, err := realtime.UpgradeWebSocket(c.Writer, c.Request)
	if err != nil {
		lg.Warn("project.ws_upgrade_failed", zap.Int("project_id", projectID), zap.Error(err))
		return
	}

	h.hub.HandleProjectConnection(c.Request.Context(), conn, *session, cursor, lg.With(zap.Int("project_id", projectID), zap.Int("uid", claims.UID)))
}
