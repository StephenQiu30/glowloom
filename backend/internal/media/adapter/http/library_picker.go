package http

import (
	"net/url"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func libraryPickerQuery(query url.Values, page *application.LibraryQuery) error {
	page.Origin = query.Get("origin")
	for key, destination := range map[string]*[]string{"resolutions": &page.Resolutions, "ratios": &page.Ratios} {
		if query.Has(key) {
			*destination = strings.Split(query.Get(key), ",")
		}
	}
	for key, destination := range map[string]**time.Time{"created_from": &page.CreatedFrom, "created_before": &page.CreatedBefore} {
		if query.Has(key) {
			value, err := time.Parse(time.RFC3339, query.Get(key))
			if err != nil {
				return domain.ErrInvalidLibrary
			}
			*destination = &value
		}
	}
	return nil
}
