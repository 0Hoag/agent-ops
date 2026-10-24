package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func Error(c *gin.Context, status int, message string) {
	c.JSON(status, ErrorResponse{Status: status, Message: message})
}

func BadRequest(c *gin.Context, message string)    { Error(c, http.StatusBadRequest, message) }
func Unauthorized(c *gin.Context, message string)  { Error(c, http.StatusUnauthorized, message) }
func Forbidden(c *gin.Context, message string)     { Error(c, http.StatusForbidden, message) }
func NotFound(c *gin.Context, message string)      { Error(c, http.StatusNotFound, message) }
func Conflict(c *gin.Context, message string)      { Error(c, http.StatusConflict, message) }
func InternalError(c *gin.Context, message string) { Error(c, http.StatusInternalServerError, message) }
