package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"time"

	"github.com/gin-gonic/gin"
)

// DiaryHandler handles product-semantic daily note endpoints.
type DiaryHandler struct {
	svc *service.TaskService
}

// NewDiaryHandler creates a diary handler.
func NewDiaryHandler(svc *service.TaskService) *DiaryHandler {
	return &DiaryHandler{svc: svc}
}

// Today
// @Summary 打开或创建今日日记
// @Description 查找或创建当前用户的“日记”空间，并打开当天 YYYY-MM-DD.md 私人文档。
// @Tags Diary
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Resp{data=service.DiaryTodayResult} "打开成功"
// @Failure 403 {object} response.Resp "权限不足"
// @Failure 500 {object} response.Resp "系统错误"
// @Router /diary/today [post]
func (h *DiaryHandler) Today(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	result, err := h.svc.OpenTodayDiary(c.Request.Context(), lg, uid, time.Now())
	if err != nil {
		handleTaskError(c, lg, err, "diary.today.failed", start)
		return
	}

	response.Success(c, result)
}
