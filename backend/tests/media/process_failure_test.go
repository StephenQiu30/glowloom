package media_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaProcessesPreserveInfrastructureFailures(t *testing.T) {
	for _, process := range []string{"probe", "render"} {
		for _, failure := range []string{"cancelled", "missing_executable"} {
			t.Run(process+"/"+failure, func(t *testing.T) {
				file, err := os.CreateTemp(t.TempDir(), "media-*.png")
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = file.Close() }()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				want := exec.ErrNotFound
				if failure == "cancelled" {
					cancel()
					want = context.Canceled
				} else {
					t.Setenv("PATH", t.TempDir())
				}
				download := &mediaapp.Downloaded{File: file, MIMEType: "image/png"}
				if process == "probe" {
					_, err = (mediaflow.FFProber{}).Probe(ctx, download)
				} else {
					_, err = (mediaflow.FFRenderer{}).Render(ctx, download, mediaapp.ProbeResult{Kind: domain.KindImage}, "16:9")
				}
				if !errors.Is(err, want) || errors.Is(err, mediaflow.ErrUnsupportedMedia) {
					t.Fatalf("infrastructure failure = %v, want %v without permanent media classification", err, want)
				}
			})
		}
	}
}
