package response

import (
	apperrors "ToDoList/server/errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Resp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Resp{
		Code:    int(apperrors.CodeOK),
		Message: "操作成功",
		Data:    data,
	})
}

func SuccessWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, Resp{
		Code:    int(apperrors.CodeOK),
		Message: msg,
		Data:    data,
	})
}

func Error(c *gin.Context, err error) {
	var e *apperrors.Error
	if apperrors.As(err, &e) {
		httpStatus := http.StatusOK
		switch e.Code {
		case apperrors.CodeParamInvalid:
			httpStatus = http.StatusBadRequest
		case apperrors.CodeUnauthorized:
			httpStatus = http.StatusUnauthorized
		case apperrors.CodeForbidden:
			httpStatus = http.StatusForbidden
		case apperrors.CodeNotFound:
			httpStatus = http.StatusNotFound
		case apperrors.CodeInternal:
			httpStatus = http.StatusInternalServerError
		case apperrors.CodeConflict:
			httpStatus = http.StatusConflict
		}
		c.JSON(httpStatus, Resp{
			Code:    int(e.Code),
			Message: e.Message,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, Resp{
		Code:    int(apperrors.CodeInternal),
		Message: "系统错误，请稍后重试",
	})
}

func ErrorWithStatus(c *gin.Context, httpStatus int, err error) {
	var e *apperrors.Error
	if apperrors.As(err, &e) {
		c.JSON(httpStatus, Resp{
			Code:    int(e.Code),
			Message: e.Message,
		})
		return
	}

	c.JSON(httpStatus, Resp{
		Code:    int(apperrors.CodeInternal),
		Message: "系统错误，请稍后重试",
	})
}

func ParamError(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, Resp{
		Code:    int(apperrors.CodeParamInvalid),
		Message: msg,
	})
}

func Unauthorized(c *gin.Context, msg string) {
	if msg == "" {
		msg = "未授权"
	}
	c.JSON(http.StatusUnauthorized, Resp{
		Code:    int(apperrors.CodeUnauthorized),
		Message: msg,
	})
}

func Forbidden(c *gin.Context, msg string) {
	if msg == "" {
		msg = "权限不足"
	}
	c.JSON(http.StatusForbidden, Resp{
		Code:    int(apperrors.CodeForbidden),
		Message: msg,
	})
}

func NotFound(c *gin.Context, msg string) {
	if msg == "" {
		msg = "资源不存在"
	}
	c.JSON(http.StatusNotFound, Resp{
		Code:    int(apperrors.CodeNotFound),
		Message: msg,
	})
}

func Conflict(c *gin.Context, msg string) {
	if msg == "" {
		msg = "资源已存在"
	}
	c.JSON(http.StatusConflict, Resp{
		Code:    int(apperrors.CodeConflict),
		Message: msg,
	})
}

type PageResult struct {
	List  interface{} `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func PageData(c *gin.Context, list interface{}, total int64, page, size int) {
	Success(c, PageResult{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	})
}
