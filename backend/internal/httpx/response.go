package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIError 统一错误体：{"code","message"} + HTTP 状态码。
// code 复用 legacy 消息键（invalid_user_auth / not_user_city / server_is_updating ...）以便前端 i18n。
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string { return e.Message }

func NewError(status int, code, message string) *APIError {
	return &APIError{Code: code, Message: message, Status: status}
}

func BadRequest(code, message string) *APIError {
	return NewError(http.StatusBadRequest, code, message)
}

func Unauthorized(code, message string) *APIError {
	return NewError(http.StatusUnauthorized, code, message)
}

func Forbidden(code, message string) *APIError {
	return NewError(http.StatusForbidden, code, message)
}

func NotFound(code, message string) *APIError {
	return NewError(http.StatusNotFound, code, message)
}

// WriteError 写出统一错误响应；非 APIError 归为 500。
func WriteError(c *gin.Context, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		c.JSON(apiErr.Status, gin.H{"code": apiErr.Code, "message": apiErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "internal_error", "message": err.Error()})
}