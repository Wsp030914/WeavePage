package handler

import (
	"ToDoList/server/realtime"
	"ToDoList/server/response"
	"time"

	"github.com/gin-gonic/gin"
)

type RealtimeMetricsHandler struct {
	projectHub *realtime.ProjectHub
	contentHub *realtime.ContentHub
}

type RealtimeMetricsResponse struct {
	CollectedAt time.Time           `json:"collected_at"`
	Project     realtime.HubMetrics `json:"project"`
	Content     realtime.HubMetrics `json:"content"`
}

func NewRealtimeMetricsHandler(projectHub *realtime.ProjectHub, contentHub *realtime.ContentHub) *RealtimeMetricsHandler {
	return &RealtimeMetricsHandler{projectHub: projectHub, contentHub: contentHub}
}

// Snapshot
// @Summary Get realtime link metrics
// @Description Returns local-node metrics for project event and document content WebSocket hubs.
// @Tags Realtime
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Resp{data=handler.RealtimeMetricsResponse} "Realtime metrics loaded"
// @Router /realtime/metrics [get]
func (h *RealtimeMetricsHandler) Snapshot(c *gin.Context) {
	response.Success(c, RealtimeMetricsResponse{
		CollectedAt: time.Now().UTC(),
		Project:     h.projectHub.MetricsSnapshot(),
		Content:     h.contentHub.MetricsSnapshot(),
	})
}
