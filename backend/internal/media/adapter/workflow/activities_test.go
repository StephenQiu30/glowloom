package workflow

import (
	"errors"
	"fmt"
	"testing"

	"go.temporal.io/sdk/temporal"
)

func TestClassifyIngestErrorSeparatesPermanentResultsFromTransientFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code string
	}{
		{name: "unsupported media", err: ErrUnsupportedMedia, code: "unsupported_media"},
		{name: "unsafe URL", err: ErrUnsafeResultURL, code: "provider_result_invalid"},
		{name: "expired URL", err: ErrResultExpired, code: "result_expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			permanent := classifyIngestError(fmt.Errorf("ingest: %w", test.err))
			var appErr *temporal.ApplicationError
			if !errors.As(permanent, &appErr) || !appErr.NonRetryable() || appErr.Type() != test.code ||
				!errors.Is(permanent, test.err) {
				t.Fatalf("permanent media classification = %v", permanent)
			}
		})
	}
	var appErr *temporal.ApplicationError
	transient := errors.New("temporary object storage failure")
	if got := classifyIngestError(transient); !errors.Is(got, transient) || errors.As(got, &appErr) {
		t.Fatalf("transient error was changed: %v", got)
	}
	if got := classifyIngestError(ErrResultUnavailable); !errors.Is(got, ErrResultUnavailable) || errors.As(got, &appErr) {
		t.Fatalf("result unavailable was marked permanent: %v", got)
	}
}
