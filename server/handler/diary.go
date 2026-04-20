package handler

import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"time"

	"github.com/gin-gonic/gin"
)

type DiaryHandler struct {
	svc *service.TaskService
}

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

// OpenDate
// @Summary 打开或创建指定日期日记
// @Description 查找或创建当前用户指定日期 YYYY-MM-DD.md 私人文档。
// @Tags Diary
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param date path string true "YYYY-MM-DD"
// @Success 200 {object} response.Resp{data=service.DiaryTodayResult} "打开成功"
// @Router /diary/{date} [post]
func (h *DiaryHandler) OpenDate(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	result, err := h.svc.OpenDiaryByDate(c.Request.Context(), lg, uid, c.Param("date"))
	if err != nil {
		handleTaskError(c, lg, err, "diary.open_date.failed", start)
		return
	}
	response.Success(c, result)
}
