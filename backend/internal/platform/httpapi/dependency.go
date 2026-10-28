package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// LogDependencyError records safe diagnostics without SQL, values or connection strings.
func LogDependencyError(c *gin.Context, err error) {
	var state interface{ SQLState() string }
	code := ""
	if errors.As(err, &state) {
		code = state.SQLState()
	}
	slog.ErrorContext(c.Request.Context(), "request dependency failed",
		"request_id", c.GetString("request_id"), "route", c.FullPath(),
		"error_type", fmt.Sprintf("%T", err), "sqlstate", code,
		"canceled", errors.Is(err, context.Canceled),
		"deadline_exceeded", errors.Is(err, context.DeadlineExceeded))
}
