package videodepth

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// Config contains operator-owned offline runtime locations, never browser input.
type Config struct {
	PythonPath  string
	SourceDir   string
	ModelPath   string
	Device      string
	FFmpegPath  string
	FFprobePath string
	// RSSLimitBytes may tighten the fixed 8 GiB sampled budget; zero uses it.
	RSSLimitBytes int64
}

const profileID = "vda-small-relative-v1"
const vdaCommit = "4f5ae23172ba60fd7bc11ef671cca678842c7072"
const modelSHA256 = "13379300b739e659f076a59d52e9801bd8d38c541a7e71f73bbca4dcfb013609"
const modelBytes int64 = 116440756

//go:embed worker.py source_manifest.json
var nativeFiles embed.FS

// Runner owns one cancelable native inference slot and checked local ML tools.
type Runner struct {
	cfg     Config
	prober  mediaapp.Prober
	prepare *Preprocessor
	slot    chan struct{}
}

// NewRunner checks offline configuration without loading a GPU or downloading.
func NewRunner(cfg Config, prober mediaapp.Prober) (*Runner, error) {
	if prober == nil || (cfg.Device != "mps" && cfg.Device != "cpu") || !filepath.IsAbs(cfg.PythonPath) || !filepath.IsAbs(cfg.SourceDir) || !filepath.IsAbs(cfg.ModelPath) {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	if cfg.Device == "mps" && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	if cfg.RSSLimitBytes == 0 {
		cfg.RSSLimitBytes = maxProcessRSS
	}
	if cfg.RSSLimitBytes < 1<<20 || cfg.RSSLimitBytes > maxProcessRSS {
		return nil, application.ErrDepthBudgetExceeded
	}
	python, err := exec.LookPath(cfg.PythonPath)
	if err != nil {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	cfg.PythonPath = python
	info, err := os.Lstat(cfg.ModelPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != modelBytes {
		return nil, application.ErrDepthModelMismatch
	}
	if info, err := os.Stat(filepath.Join(cfg.SourceDir, "video_depth_anything", "video_depth.py")); err != nil || !info.Mode().IsRegular() {
		return nil, application.ErrDepthModelMismatch
	}
	if info, err := os.Lstat(cfg.SourceDir); err != nil || !info.IsDir() {
		return nil, application.ErrDepthModelMismatch
	}
	prepare, err := NewPreprocessor(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		return nil, err
	}
	return &Runner{cfg: cfg, prober: prober, prepare: prepare, slot: make(chan struct{}, 1)}, nil
}

// Process verifies input/runtime before actual inference and returns a checked
// private MP4 only after every native process has stopped. It does not publish.
func (r *Runner) Process(ctx context.Context, source *mediaapp.Downloaded, sha string, report func(application.DepthPhase) error) (result *application.DepthOutput, resultErr error) {
	if r == nil {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	select {
	case r.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() {
		if !errors.Is(resultErr, application.ErrDepthCessationUncertain) {
			<-r.slot
		}
	}()
	phase := func(value application.DepthPhase) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if report != nil {
			return report(value)
		}
		return nil
	}
	if err := phase(application.DepthChecking); err != nil {
		return nil, err
	}
	tree, err := r.validateBundle(ctx)
	if err != nil {
		return nil, err
	}
	if err := phase(application.DepthPreparing); err != nil {
		return nil, err
	}
	prepared, err := r.prepare.Prepare(ctx, source, sha)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errors.Is(resultErr, application.ErrDepthCessationUncertain) {
			// Retain the owned file while a child may still read it. The poisoned
			// runner cannot admit another job; operator reconciliation is required.
			_ = prepared.File.File.Close()
		} else {
			_ = prepared.File.Close()
		}
	}()
	dir, err := os.MkdirTemp("", "lanverse-depth-native-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if !errors.Is(resultErr, application.ErrDepthCessationUncertain) {
			_ = os.RemoveAll(dir)
		}
	}()
	script, err := nativeFiles.ReadFile("worker.py")
	if err != nil {
		return nil, err
	}
	worker := filepath.Join(dir, "worker.py")
	if err := os.WriteFile(worker, script, 0600); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "lanverse-depth-output-*.mp4")
	if err != nil {
		return nil, err
	}
	output := &mediaapp.Downloaded{File: file, MIMEType: "video/mp4"}
	keep := false
	defer func() {
		if !keep {
			if errors.Is(resultErr, application.ErrDepthCessationUncertain) {
				_ = output.File.Close()
			} else {
				_ = output.Close()
			}
		}
	}()
	var complete struct {
		Complete       bool    `json:"complete"`
		Device         string  `json:"device"`
		RuntimeVersion string  `json:"runtime_version"`
		FrameCount     int     `json:"frame_count"`
		FPS            float64 `json:"fps"`
	}
	var failure string
	args := []string{"-I", "-B", worker, "--input", prepared.File.File.Name(), "--output", file.Name(), "--source", r.cfg.SourceDir, "--model", r.cfg.ModelPath, "--device", r.cfg.Device, "--ffmpeg", r.prepare.ffmpeg}
	nativeCtx, stop := context.WithTimeout(ctx, 30*time.Minute)
	defer stop()
	_, native, err := runProcess(nativeCtx, r.cfg.PythonPath, args, r.cfg.RSSLimitBytes, func(line []byte) error {
		var event struct {
			Phase    application.DepthPhase `json:"phase"`
			Failure  string                 `json:"failure_code"`
			Complete bool                   `json:"complete"`
		}
		if json.Unmarshal(line, &event) != nil {
			return application.ErrDepthOutputInvalid
		}
		if event.Failure != "" {
			failure = event.Failure
			return nil
		}
		if event.Complete {
			return json.Unmarshal(line, &complete)
		}
		switch event.Phase {
		case application.DepthLoading, application.DepthInferring, application.DepthEncoding:
			return phase(event.Phase)
		default:
			return application.ErrDepthOutputInvalid
		}
	})
	if err != nil {
		if errors.Is(err, application.ErrDepthCessationUncertain) {
			return nil, err
		}
		if nativeCtx.Err() != nil {
			return nil, errors.Join(nativeCtx.Err(), err)
		}
		if failure != "" {
			return nil, depthFailure(failure)
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, application.ErrDepthInferenceFailed
		}
		return nil, err
	}
	if !complete.Complete || complete.Device != r.cfg.Device || complete.RuntimeVersion == "" || complete.FrameCount != prepared.Prepared.FrameCount || !finite(complete.FPS) || math.Abs(complete.FPS-prepared.Prepared.FPS) > 0.001 {
		return nil, application.ErrDepthOutputInvalid
	}
	if err := phase(application.DepthVerifying); err != nil {
		return nil, err
	}
	verify, stopVerify := context.WithTimeout(ctx, 2*time.Minute)
	defer stopVerify()
	facts, err := r.prepare.ProbeVideo(verify, file.Name())
	if err != nil {
		return nil, err
	}
	if facts.Width != 1920 || facts.Height != 1080 || facts.FrameCount != prepared.Prepared.FrameCount || math.Abs(facts.FPS-prepared.Prepared.FPS) > 0.001 || absMS(facts.DurationMS-prepared.Prepared.DurationMS) > int64(math.Ceil(1000/facts.FPS))+2 {
		return nil, application.ErrDepthOutputInvalid
	}
	if err := r.prepare.VerifyDecode(verify, file.Name()); err != nil {
		return nil, err
	}
	if err := r.prepare.VerifyCanonicalOutput(verify, file.Name()); err != nil {
		return nil, err
	}
	if err := finishFile(output); err != nil {
		return nil, err
	}
	probe, err := r.prober.Probe(verify, output)
	if err != nil {
		return nil, errors.Join(application.ErrDepthOutputInvalid, err)
	}
	if probe.Codec == nil || *probe.Codec != "h264" || probe.Width == nil || *probe.Width != 1920 || probe.Height == nil || *probe.Height != 1080 || probe.DurationMS == nil || absMS(int64(*probe.DurationMS)-facts.DurationMS) > 2 {
		return nil, application.ErrDepthOutputInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keep = true
	return &application.DepthOutput{File: output, Probe: probe, Receipt: application.DepthReceipt{ProfileID: profileID, VDACommit: vdaCommit, ModelSHA256: modelSHA256, SourceTreeSHA256: tree, RuntimeVersion: complete.RuntimeVersion, Device: complete.Device, InputSHA256: sha, Source: prepared.Source, Prepared: prepared.Prepared, Output: facts, Native: native}}, nil
}

func (r *Runner) validateBundle(ctx context.Context) (string, error) {
	model, err := os.Open(r.cfg.ModelPath)
	if err != nil {
		return "", application.ErrDepthModelMismatch
	}
	defer func() { _ = model.Close() }()
	info, err := model.Stat()
	if err != nil || info.Size() != modelBytes {
		return "", application.ErrDepthModelMismatch
	}
	if err := checkDigest(ctx, model, modelBytes, modelSHA256); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", application.ErrDepthModelMismatch
	}
	data, err := nativeFiles.ReadFile("source_manifest.json")
	if err != nil {
		return "", err
	}
	var manifest struct {
		Commit string            `json:"vda_commit"`
		Files  map[string]string `json:"files"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Commit != vdaCommit {
		return "", application.ErrDepthModelMismatch
	}
	for name, digest := range manifest.Files {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		path := filepath.Join(r.cfg.SourceDir, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
			return "", application.ErrDepthModelMismatch
		}
		file, err := os.Open(path)
		if err != nil {
			return "", application.ErrDepthModelMismatch
		}
		err = checkDigest(ctx, file, info.Size(), digest)
		_ = file.Close()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", application.ErrDepthModelMismatch
		}
	}
	err = filepath.WalkDir(r.cfg.SourceDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.Name() == "__pycache__" {
			return application.ErrDepthModelMismatch
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".py") {
			rel, err := filepath.Rel(r.cfg.SourceDir, path)
			if err != nil {
				return err
			}
			if _, ok := manifest.Files[filepath.ToSlash(rel)]; !ok {
				return application.ErrDepthModelMismatch
			}
		}
		for _, suffix := range []string{".pyc", ".pyo", ".so", ".dylib"} {
			if strings.HasSuffix(entry.Name(), suffix) {
				return application.ErrDepthModelMismatch
			}
		}
		return nil
	})
	if err != nil {
		return "", errors.Join(application.ErrDepthModelMismatch, err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func depthFailure(code string) error {
	switch code {
	case "depth_runtime_unavailable":
		return application.ErrDepthRuntimeUnavailable
	case "depth_device_unavailable":
		return application.ErrDepthDeviceUnavailable
	case "depth_budget_exceeded":
		return application.ErrDepthBudgetExceeded
	case "depth_input_invalid":
		return application.ErrInvalidDepthInput
	case "depth_output_invalid":
		return application.ErrDepthOutputInvalid
	default:
		return application.ErrDepthInferenceFailed
	}
}

var _ application.DepthProcessor = (*Runner)(nil)
