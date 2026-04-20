package handler

// 文件说明：这个文件负责文档评论相关的 HTTP 接口。
// 实现方式：在接口层完成参数绑定和错误映射，权限判定与状态流转交给评论 service。
// 这样做的好处是评论能力可以复用任务正文会话的权限模型，同时保持接口层简单。
import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateTaskCommentReq struct {
	ContentMD  string `json:"content_md" binding:"required"`
	AnchorType string `json:"anchor_type"`
	AnchorText string `json:"anchor_text"`
}

type UpdateTaskCommentReq struct {
	Resolved *bool `json:"resolved"`
}

type TaskCommentHandler struct {
	svc *service.TaskCommentService
}

// NewTaskCommentHandler 创建评论接口处理器。
func NewTaskCommentHandler(svc *service.TaskCommentService) *TaskCommentHandler {
	return &TaskCommentHandler{svc: svc}
}

// List
// @Summary List document comments
// @Description Returns document-level comments for a document, meeting note, or todo. Diary documents are rejected.
// @Tags Comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Document ID"
// @Success 200 {object} response.Resp{data=[]models.TaskCommentInfo} "Comments"
// @Failure 403 {object} response.Resp "Forbidden"
// @Failure 404 {object} response.Resp "Document not found"
// @Router /documents/{id}/comments [get]
func (h *TaskCommentHandler) List(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid document id")
		return
	}

	comments, err := h.svc.ListByTask(c.Request.Context(), lg, uid, taskID)
	if err != nil {
		handleTaskError(c, lg, err, "task_comment.list.failed", start)
		return
	}

	response.Success(c, comments)
}

// Create 创建一条文档级评论。
// 评论接口复用了文档权限会话，因此会议纪要和协作文档天然继承同一套可见性规则。
// @Summary Create a document comment
// @Description Creates a document-level comment for a document, meeting note, or todo. Diary documents are rejected.
// @Tags Comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Document ID"
// @Param req body CreateTaskCommentReq true "Comment payload"
// @Success 200 {object} response.Resp{data=models.TaskCommentInfo} "Created"
// @Failure 403 {object} response.Resp "Forbidden"
// @Failure 404 {object} response.Resp "Document not found"
// @Router /documents/{id}/comments [post]
func (h *TaskCommentHandler) Create(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid document id")
		return
	}

	var req CreateTaskCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	comment, err := h.svc.Create(c.Request.Context(), lg, uid, taskID, service.CreateTaskCommentInput{
		ContentMD:  req.ContentMD,
		AnchorType: req.AnchorType,
		AnchorText: req.AnchorText,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task_comment.create.failed", start)
		return
	}

	response.Success(c, comment)
}

// Update 更新评论状态。
// 当前只暴露 resolved 字段，是为了先把“讨论关闭”这条主流程稳定下来，避免过早开放复杂编辑模型。
// @Summary Update a comment state
// @Description Updates comment state such as resolved/unresolved. Authors and document owners/editors can resolve comments.
// @Tags Comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Comment ID"
// @Param req body UpdateTaskCommentReq true "Comment patch"
// @Success 200 {object} response.Resp{data=models.TaskCommentInfo} "Updated"
// @Failure 403 {object} response.Resp "Forbidden"
// @Failure 404 {object} response.Resp "Comment not found"
// @Router /comments/{id} [patch]
func (h *TaskCommentHandler) Update(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid comment id")
		return
	}

	var req UpdateTaskCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "invalid request body")
		return
	}

	comment, err := h.svc.Update(c.Request.Context(), lg, uid, commentID, service.UpdateTaskCommentInput{
		Resolved: req.Resolved,
	})
	if err != nil {
		handleTaskError(c, lg, err, "task_comment.update.failed", start)
		return
	}

	response.Success(c, comment)
}

// Delete 删除一条评论。
// 作者本人和文档 owner/editor 都可以删除，是为了兼顾个人撤回和协作者治理两类场景。
// @Summary Delete a comment
// @Description Deletes a comment. The author and document owners/editors can delete comments.
// @Tags Comment
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Comment ID"
// @Success 200 {object} response.Resp "Deleted"
// @Failure 403 {object} response.Resp "Forbidden"
// @Failure 404 {object} response.Resp "Comment not found"
// @Router /comments/{id} [delete]
func (h *TaskCommentHandler) Delete(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	commentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ParamError(c, "invalid comment id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), lg, uid, commentID); err != nil {
		handleTaskError(c, lg, err, "task_comment.delete.failed", start)
		return
	}

	response.SuccessWithMsg(c, "comment deleted", nil)
}
