package handler

// 文件说明：这个文件负责某类 HTTP 接口的参数绑定、上下文提取与错误映射。
// 实现方式：接口层尽量保持薄，只做协议转换并调用服务层。
// 这样做的好处是业务规则集中在 service 层，接口层更容易维护。
import (
	"ToDoList/server/response"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type DocumentImportHandler struct {
	svc *service.DocumentImportService
}

func NewDocumentImportHandler(svc *service.DocumentImportService) *DocumentImportHandler {
	return &DocumentImportHandler{svc: svc}
}

type CreateDocumentImportReq struct {
	ProjectID         int    `json:"project_id" binding:"required"`
	FileName          string `json:"file_name" binding:"required"`
	Title             string `json:"title"`
	TotalSize         int64  `json:"total_size" binding:"required"`
	TotalParts        int    `json:"total_parts"`
	ChunkSize         int64  `json:"chunk_size"`
	SHA256            string `json:"sha256"`
	CollaborationMode string `json:"collaboration_mode"`
}

type CompleteDocumentImportReq struct {
	Title string `json:"title"`
}

// CreateSession
// @Summary Create Markdown document import session
// @Description Creates a resumable Markdown import session. Use collaboration_mode=collaborative for shared docs or private for private docs.
// @Tags DocumentImport
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param req body CreateDocumentImportReq true "Import metadata"
// @Success 200 {object} response.Resp{data=service.DocumentImportSessionResult} "Session created"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Router /documents/imports [post]
func (h *DocumentImportHandler) CreateSession(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req CreateDocumentImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, "参数格式有误")
		return
	}

	result, err := h.svc.CreateSession(c.Request.Context(), lg, uid, service.CreateDocumentImportInput{
		ProjectID:         req.ProjectID,
		FileName:          req.FileName,
		Title:             req.Title,
		TotalSize:         req.TotalSize,
		TotalParts:        req.TotalParts,
		ChunkSize:         req.ChunkSize,
		SHA256:            req.SHA256,
		CollaborationMode: req.CollaborationMode,
	})
	if err != nil {
		handleTaskError(c, lg, err, "document_import.create_session.failed", start)
		return
	}
	response.Success(c, result)
}

// UploadPart
// @Summary Upload Markdown import chunk
// @Description Uploads one raw binary Markdown chunk for a resumable import session.
// @Tags DocumentImport
// @Accept octet-stream
// @Produce json
// @Security BearerAuth
// @Param upload_id path string true "Upload session id"
// @Param part_no path int true "1-based part number"
// @Param file body string true "Raw chunk bytes"
// @Success 200 {object} response.Resp{data=service.DocumentImportPartResult} "Part uploaded"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Session not found"
// @Failure 409 {object} response.Resp "Session busy or incomplete"
// @Router /documents/imports/{upload_id}/parts/{part_no} [put]
func (h *DocumentImportHandler) UploadPart(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	partNo, err := strconv.Atoi(c.Param("part_no"))
	if err != nil {
		response.ParamError(c, "invalid part_no")
		return
	}
	size := c.Request.ContentLength
	if size <= 0 {
		response.ParamError(c, "empty part")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.DocumentImportMaxPartBytes+1)

	result, err := h.svc.UploadPart(c.Request.Context(), lg, uid, c.Param("upload_id"), partNo, c.Request.Body, size)
	if err != nil {
		handleTaskError(c, lg, err, "document_import.upload_part.failed", start)
		return
	}
	response.Success(c, result)
}

// UploadAsset
// @Summary Upload Markdown referenced image
// @Description Uploads an image asset referenced by the Markdown file and records its original relative path for link rewriting.
// @Tags DocumentImport
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param upload_id path string true "Upload session id"
// @Param original_path formData string false "Original relative path in Markdown, such as images/a.png"
// @Param file formData file true "Image file"
// @Success 200 {object} response.Resp{data=service.DocumentImportAssetResult} "Asset uploaded"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Session not found"
// @Router /documents/imports/{upload_id}/assets [post]
func (h *DocumentImportHandler) UploadAsset(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.DocumentImportMaxAssetBytes+1024*1024)
	file, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			response.ParamError(c, "file is required")
			return
		}
		response.ParamError(c, "failed to read file")
		return
	}

	result, err := h.svc.UploadAsset(c.Request.Context(), lg, uid, c.Param("upload_id"), c.PostForm("original_path"), file)
	if err != nil {
		handleTaskError(c, lg, err, "document_import.upload_asset.failed", start)
		return
	}
	response.Success(c, result)
}

// Complete
// @Summary Complete Markdown document import
// @Description Assembles uploaded chunks, rewrites local image references to uploaded asset URLs, and creates a document.
// @Tags DocumentImport
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param upload_id path string true "Upload session id"
// @Param req body CompleteDocumentImportReq false "Optional completion data"
// @Success 200 {object} response.Resp{data=service.DocumentImportCompleteResult} "Document imported"
// @Failure 400 {object} response.Resp "Invalid request parameters"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Session not found"
// @Failure 409 {object} response.Resp "Upload incomplete or digest mismatch"
// @Router /documents/imports/{upload_id}/complete [post]
func (h *DocumentImportHandler) Complete(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	var req CompleteDocumentImportReq
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ParamError(c, "参数格式有误")
			return
		}
	}

	result, err := h.svc.Complete(c.Request.Context(), lg, uid, c.Param("upload_id"), service.CompleteDocumentImportInput{
		Title: req.Title,
	})
	if err != nil {
		handleTaskError(c, lg, err, "document_import.complete.failed", start)
		return
	}
	response.Success(c, result)
}

// Abort
// @Summary Abort Markdown document import
// @Description Deletes temporary uploaded chunks and assets for an unfinished import session.
// @Tags DocumentImport
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param upload_id path string true "Upload session id"
// @Success 200 {object} response.Resp "Import aborted"
// @Failure 403 {object} response.Resp "Permission denied"
// @Failure 404 {object} response.Resp "Session not found"
// @Router /documents/imports/{upload_id} [delete]
func (h *DocumentImportHandler) Abort(c *gin.Context) {
	lg := utils.CtxLogger(c)
	start := time.Now()
	uid := c.GetInt("uid")

	if err := h.svc.Abort(c.Request.Context(), lg, uid, c.Param("upload_id")); err != nil {
		handleTaskError(c, lg, err, "document_import.abort.failed", start)
		return
	}
	response.Success(c, nil)
}
