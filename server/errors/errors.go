package errors

// 文件说明：这个文件负责定义应用层错误模型。
// 实现方式：统一封装参数错误、权限错误、冲突错误和内部错误。
// 这样做的好处是业务错误到 HTTP 响应的映射更加稳定。
import (
	"errors"
	"fmt"
)

type Code int

const (
	CodeOK           Code = 0
	CodeInternal     Code = 5001
	CodeParamInvalid Code = 4001
	CodeUnauthorized Code = 4002
	CodeForbidden    Code = 4003
	CodeNotFound     Code = 4004
	CodeConflict     Code = 4009
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return nil
}

func NewError(code Code, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

func NewInternalError(msg string) *Error {
	return &Error{Code: CodeInternal, Message: msg}
}

func NewParamError(msg string) *Error {
	return &Error{Code: CodeParamInvalid, Message: msg}
}

func NewUnauthorizedError(msg string) *Error {
	return &Error{Code: CodeUnauthorized, Message: msg}
}

func NewForbiddenError(msg string) *Error {
	return &Error{Code: CodeForbidden, Message: msg}
}

func NewNotFoundError(msg string) *Error {
	return &Error{Code: CodeNotFound, Message: msg}
}

func NewConflictError(msg string) *Error {
	return &Error{Code: CodeConflict, Message: msg}
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

var (
	ErrNotFound     = NewNotFoundError("not found")
	ErrConflict     = NewConflictError("conflict")
	ErrUnauthorized = NewUnauthorizedError("unauthorized")
)

func Wrap(err error, msg string) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{Code: CodeInternal, Message: fmt.Sprintf("%s: %s", msg, err.Error())}
}

func As(err error, target interface{}) bool {
	if err == nil || target == nil {
		return false
	}
	return errors.As(err, target)
}
