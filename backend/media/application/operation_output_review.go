package application

import (
	"context"
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ProviderOutputReviewer applies the existing provider-moderation contract only
// after Media's durable prepare verified the original Execution proof, including
// frozen provider policy and succeeded paid manifest. It infers no platform review.
type ProviderOutputReviewer struct{}

// NewProviderOutputReviewer constructs Media's explicit provider-policy reviewer.
func NewProviderOutputReviewer() *ProviderOutputReviewer { return &ProviderOutputReviewer{} }

// ReviewOutput records Media's decision for these exact archived bytes. The
// SourceVerified fact comes from Media's durable prepare, never a caller header;
// expiry of the old Execution lease cannot erase an already admitted archive.
func (r *ProviderOutputReviewer) ReviewOutput(_ context.Context, work OperationOutputWork, asset domain.MediaAsset) (OutputReview, error) {
	if r == nil || !work.SourceVerified || ValidateOutputPublication(work.Request) != nil || asset.ID != work.AssetID || asset.SourceOperationID == nil || *asset.SourceOperationID != work.Request.Output.OperationID || asset.SHA256 == nil || *asset.SHA256 != work.Request.Output.SHA256 || asset.ByteSize != work.Request.Output.ByteSize {
		return OutputReview{}, domain.ErrOutputPublicationConflict
	}
	raw, err := json.Marshal(struct {
		Source         string `json:"source"`
		ManifestSHA256 string `json:"manifest_sha256"`
		OutputSHA256   string `json:"output_sha256"`
	}{"provider", work.Request.Output.Provider.ManifestSHA256, work.Request.Output.SHA256})
	if err != nil {
		return OutputReview{}, err
	}
	return OutputReview{Status: "passed", Detail: raw}, nil
}
