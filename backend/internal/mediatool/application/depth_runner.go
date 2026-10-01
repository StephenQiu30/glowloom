package application

import (
	"context"
	"errors"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

// ErrInvalidDepthInput and the remaining errors preserve native failure classes.
var (
	ErrInvalidDepthInput       = errors.New("depth input invalid")
	ErrDepthRuntimeUnavailable = errors.New("depth runtime unavailable")
	ErrDepthModelMismatch      = errors.New("depth model mismatch")
	ErrDepthBudgetExceeded     = errors.New("depth budget exceeded")
	ErrDepthDeviceUnavailable  = errors.New("depth device unavailable")
	ErrDepthOutputInvalid      = errors.New("depth output invalid")
	ErrDepthInferenceFailed    = errors.New("depth inference failed")
	ErrDepthCessationUncertain = errors.New("depth process cessation uncertain")
)

// DepthPhase reports actual work without fabricated frame progress.
type DepthPhase string

// DepthChecking and subsequent phases describe actual native work.
const (
	DepthChecking  DepthPhase = "checking"
	DepthPreparing DepthPhase = "preparing"
	DepthLoading   DepthPhase = "loading"
	DepthInferring DepthPhase = "inferring"
	DepthEncoding  DepthPhase = "encoding"
	DepthVerifying DepthPhase = "verifying"
)

// DepthVideoFacts describes fully decoded bytes, rather than a source claim.
type DepthVideoFacts struct {
	Width      int32   `json:"width"`
	Height     int32   `json:"height"`
	FrameCount int     `json:"frame_count"`
	FPS        float64 `json:"fps"`
	DurationMS int64   `json:"duration_ms"`
}

// PreparedDepthVideo owns a checked CFR file until its caller closes File.
type PreparedDepthVideo struct {
	File     *mediaapp.Downloaded
	Source   DepthVideoFacts
	Prepared DepthVideoFacts
}

// DepthExecutionFacts records bounded native observations, not an OS memory cap.
type DepthExecutionFacts struct {
	ElapsedMS          int64 `json:"elapsed_ms"`
	PeakRSSBytes       int64 `json:"peak_rss_bytes"`
	RSSLimitBytes      int64 `json:"rss_limit_bytes"`
	ProcessGroupJoined bool  `json:"process_group_joined"`
}

// DepthReceipt contains only checked native facts; it never carries local paths.
type DepthReceipt struct {
	ProfileID        string              `json:"profile_id"`
	VDACommit        string              `json:"vda_commit"`
	ModelSHA256      string              `json:"model_sha256"`
	SourceTreeSHA256 string              `json:"source_tree_sha256"`
	RuntimeVersion   string              `json:"runtime_version"`
	Device           string              `json:"device"`
	InputSHA256      string              `json:"input_sha256"`
	Source           DepthVideoFacts     `json:"source"`
	Prepared         DepthVideoFacts     `json:"prepared"`
	Output           DepthVideoFacts     `json:"output"`
	Native           DepthExecutionFacts `json:"native"`
}

// DepthOutput transfers the real MP4's cleanup ownership to the consumer.
type DepthOutput struct {
	File    *mediaapp.Downloaded
	Probe   mediaapp.ProbeResult
	Receipt DepthReceipt
}

// DepthProcessor consumes a worker-owned original and stops on context or phase
// callback errors. Implementations must wait for their entire process tree before
// returning; success alone does not publish or review a formal media asset.
type DepthProcessor interface {
	Process(context.Context, *mediaapp.Downloaded, string, func(DepthPhase) error) (*DepthOutput, error)
}
