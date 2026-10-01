package videodepth

import (
	"context"
	"time"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// Verify independently checks a consumer-owned result after native inference.
// It uses the same joined process-group lifecycle without trusting its receipt.
func (p *Preprocessor) Verify(ctx context.Context, file *mediaapp.Downloaded) (application.DepthVideoFacts, error) {
	if p == nil || file == nil || file.File == nil || file.Size < 1 || file.Size > 500<<20 || file.MIMEType != "video/mp4" {
		return application.DepthVideoFacts{}, application.ErrDepthOutputInvalid
	}
	if err := checkDigest(ctx, file.File, file.Size, file.SHA256); err != nil {
		return application.DepthVideoFacts{}, err
	}
	verify, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()
	facts, err := p.ProbeVideo(verify, file.File.Name())
	if err != nil {
		return application.DepthVideoFacts{}, err
	}
	if facts.Width != 1920 || facts.Height != 1080 || facts.FrameCount < 1 || facts.FrameCount > 453 || facts.FPS > 30 || facts.DurationMS > 15100 {
		return application.DepthVideoFacts{}, application.ErrDepthOutputInvalid
	}
	if err := p.VerifyDecode(verify, file.File.Name()); err != nil {
		return application.DepthVideoFacts{}, err
	}
	if err := p.VerifyCanonicalOutput(verify, file.File.Name()); err != nil {
		return application.DepthVideoFacts{}, err
	}
	return facts, nil
}

var _ application.DepthVerifier = (*Preprocessor)(nil)
