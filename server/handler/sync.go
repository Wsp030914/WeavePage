package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type SyncHandler struct {
	svc *service.TaskService
}

func NewSyncHandler(svc *service.TaskService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// ProjectEvents
// @Summary Sync project task events
// @Description Returns task event rows after a cursor for reconnect/backfill. The cursor is the task_events auto-increment id.
// @Tags Realtime
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param cursor query int false "Last seen task event id"
// @Param limit query int false "Page size, max 200"
// @Success 200 {object} response.Resp{data=service.ProjectSyncResult} "Sync events loaded"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Project not found"
// @Router /projects/{id}/sync [get]
func (h *SyncHandler) ProjectEvents(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	projectID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid project id")
		return
	}

	cursor, err := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	if err != nil || cursor < 0 {
		response.ParamError(c, "invalid cursor")
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 {
		response.ParamError(c, "invalid limit")
		return
	}

	result, err := h.svc.SyncProjectEvents(c.Request.Context(), lg, uid, projectID, service.ProjectSyncInput{
		Cursor: cursor,
		Limit:  limit,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.sync.failed", start)
		return
	}

	response.Success(c, result)
}
