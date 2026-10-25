package middleware

import "github.com/gin-gonic/gin"

func Noop() gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
