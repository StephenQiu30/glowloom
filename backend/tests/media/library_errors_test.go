package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type unavailableLibrary struct{}

func (unavailableLibrary) ListLibrary(context.Context, identity.Principal, domain.LibraryScope, mediaapp.LibraryQuery) (mediaapp.LibraryPage, error) {
	return mediaapp.LibraryPage{}, fmt.Errorf("private diagnostic: %w", &pgconn.PgError{Code: "40001", Message: "private SQL values"})
}
func (unavailableLibrary) LibraryDetail(context.Context, identity.Principal, domain.LibraryScope, uuid.UUID) (mediaapp.LibraryItemDetail, error) {
	return mediaapp.LibraryItemDetail{}, mediaapp.ErrUnavailable
}
func (unavailableLibrary) ApplyLibraryCommand(context.Context, identity.Principal, mediaapp.LibraryCommand) (mediaapp.LibraryReceipt, error) {
	return mediaapp.LibraryReceipt{}, mediaapp.ErrUnavailable
}
func TestLibraryDependencyErrorLogsSQLStateWithoutPrivateDetails(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	router := gin.New()
	mediahttp.NewLibraryHandler(unavailableLibrary{}).Register(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/media/library?scope=personal&page=1&page_size=12", nil))
	if response.Code != 503 {
		t.Fatalf("status=%d", response.Code)
	}
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("missing structured dependency diagnostic: %v", err)
	}
	if record["sqlstate"] != "40001" {
		t.Fatalf("missing safe SQLSTATE: %v", record)
	}
	if strings.Contains(logs.String(), "private") || strings.Contains(response.Body.String(), "40001") {
		t.Fatal("private dependency details leaked")
	}
}
