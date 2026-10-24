package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type SuccessResponse struct {
	Status int `json:"status"`
	Data   any `json:"data"`
}

type MessageResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func JSON(c *gin.Context, status int, body any) { c.JSON(status, body) }

func OK(c *gin.Context, data any) { Success(c, http.StatusOK, data) }

func Created(c *gin.Context, data any) { Success(c, http.StatusCreated, data) }

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, SuccessResponse{Status: status, Data: data})
}

func Message(c *gin.Context, status int, message string) {
	c.JSON(status, MessageResponse{Status: status, Message: message})
}

func OKMessage(c *gin.Context, message string) { Message(c, http.StatusOK, message) }
