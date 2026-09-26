// Package app is the composition root: it wires routes and runs the process for each role.
package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter builds the HTTP router for the api role.
func NewRouter(logger *zap.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), requestLogger(logger))

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return router
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Debug("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
