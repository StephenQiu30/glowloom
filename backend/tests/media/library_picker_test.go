package media_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestLibraryPickerFiltersRejectUnprovenAndMalformedSemantics(t *testing.T) {
	id := uuid.New()
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &id}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := from.Add(24 * time.Hour)
	q := libraryQuery()
	q.Kind = "image"
	q.CreatedFrom = &from
	q.CreatedBefore = &before
	q.Origin = "upload"
	q.Resolutions = []string{"1K", "4K"}
	q.Ratios = []string{"16:9", "1:1"}
	if err := q.Validate(scope); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*mediaapp.LibraryQuery){
		func(q *mediaapp.LibraryQuery) { q.Kind = "audio" },
		func(q *mediaapp.LibraryQuery) { q.Origin = "super_resolution" },
		func(q *mediaapp.LibraryQuery) { q.Resolutions = []string{"1080P"} },
		func(q *mediaapp.LibraryQuery) { q.Ratios = []string{"16:9", "16:9"} },
		func(q *mediaapp.LibraryQuery) { q.Ratios = []string{"2:0"} },
		func(q *mediaapp.LibraryQuery) { q.CreatedBefore = &from },
		func(q *mediaapp.LibraryQuery) { q.FavoriteOnly = true },
	} {
		bad := q
		mutate(&bad)
		if err := bad.Validate(scope); err == nil {
			t.Fatalf("invalid picker filter accepted %+v", bad)
		}
	}
}
