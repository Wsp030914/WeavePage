package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type TaskActivityHandler struct {
	svc *service.TaskService
}

func NewTaskActivityHandler(svc *service.TaskService) *TaskActivityHandler {
	return &TaskActivityHandler{svc: svc}
}

// ProjectActivities
// @Summary List project document activities
// @Description Returns recent-first paged document activity records backed by task_events. Supports filtering by document id.
// @Tags Activity
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param cursor query int false "Oldest loaded activity id, used to load older records"
// @Param limit query int false "Page size, max 200"
// @Param task_id query int false "Document ID filter"
// @Success 200 {object} response.Resp{data=service.ProjectActivityResult} "Document activities loaded"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Project or document not found"
// @Router /projects/{id}/activities [get]
func (h *TaskActivityHandler) ProjectActivities(c *gin.Context) {
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

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 {
		response.ParamError(c, "invalid limit")
		return
	}

	taskID := 0
	if raw := c.Query("task_id"); raw != "" {
		taskID, err = strconv.Atoi(raw)
		if err != nil || taskID <= 0 {
			response.ParamError(c, "invalid task_id")
			return
		}
	}

	result, err := h.svc.ListProjectActivities(c.Request.Context(), lg, uid, projectID, service.ProjectActivityInput{
		Cursor: cursor,
		Limit:  limit,
		TaskID: taskID,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task.activity.list.failed", start)
		return
	}

	response.Success(c, result)
}
