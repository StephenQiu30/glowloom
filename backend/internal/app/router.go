// Package app is the composition root: it wires routes and runs the process for each role.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ReadyCheck tests whether dependencies needed by the API are available.
type ReadyCheck func(context.Context) error

// NewRouter builds the HTTP router for the api role.
func NewRouter(logger *zap.Logger, ready ReadyCheck) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), requestLogger(logger))

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		if ready == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
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
