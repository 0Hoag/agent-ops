package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const ContextKeyRequestID = "request_id"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(ContextKeyRequestID, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if id, ok := c.Get(ContextKeyRequestID); ok {
		if requestID, ok := id.(string); ok {
			return requestID
		}
	}
	return ""
}
