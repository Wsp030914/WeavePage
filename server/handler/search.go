package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	taskSvc *service.TaskService
}

func NewSearchHandler(taskSvc *service.TaskService) *SearchHandler {
	return &SearchHandler{taskSvc: taskSvc}
}

// Workspace searches spaces and documents in one request.
// @Summary Search workspace
// @Description Searches current user's spaces and active documents/todos.
// @Tags Search
// @Produce json
// @Security BearerAuth
// @Param q query string true "Search query"
// @Param limit query int false "Max results per category"
// @Success 200 {object} response.Resp{data=service.WorkspaceSearchResult} "Search result"
// @Router /search [get]
func (h *SearchHandler) Workspace(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	result, err := h.taskSvc.SearchWorkspace(c.Request.Context(), lg, uid, service.WorkspaceSearchInput{
		Query: c.Query("q"),
		Limit: limit,
	})
	if err != nil {
		handleTaskError(c, lg, err, "workspace.search.failed", start)
		return
	}

	response.Success(c, result)
}
