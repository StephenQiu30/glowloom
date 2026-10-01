package videodepth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// Preprocessor prepares an owned CFR original before native temporal inference.
type Preprocessor struct{ ffmpeg, ffprobe string }

// NewPreprocessor injects only the actual local AV tools used by this profile.
func NewPreprocessor(ffmpegPath, ffprobePath string) (*Preprocessor, error) {
	ffmpeg, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	ffprobe, err := exec.LookPath(ffprobePath)
	if err != nil {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	return &Preprocessor{ffmpeg: ffmpeg, ffprobe: ffprobe}, nil
}

// Prepare checks the original digest and decodes its complete timestamps into
// CFR, fixing the source VDA reader's integer-stride duration error.
func (p *Preprocessor) Prepare(ctx context.Context, source *mediaapp.Downloaded, inputSHA256 string) (result *application.PreparedDepthVideo, resultErr error) {
	if p == nil {
		return nil, application.ErrDepthRuntimeUnavailable
	}
	if source == nil || source.File == nil || source.Size < 1 || len(inputSHA256) != 64 {
		return nil, application.ErrInvalidDepthInput
	}
	if source.Size > 500<<20 {
		return nil, application.ErrDepthBudgetExceeded
	}
	if source.MIMEType != "video/mp4" && source.MIMEType != "video/quicktime" {
		return nil, application.ErrInvalidDepthInput
	}
	info, err := source.File.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.Size {
		return nil, application.ErrInvalidDepthInput
	}
	if err := checkDigest(ctx, source.File, source.Size, inputSHA256); err != nil {
		return nil, err
	}
	stage, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()
	facts, err := p.ProbeVideo(stage, source.File.Name())
	if err != nil {
		return nil, err
	}
	if facts.DurationMS > 15100 {
		return nil, application.ErrDepthBudgetExceeded
	}
	if facts.DurationMS < 1 || facts.Width < 1 || facts.Height < 1 || facts.FPS <= 0 || facts.FrameCount < 1 {
		return nil, application.ErrInvalidDepthInput
	}
	if err := p.VerifyDecode(stage, source.File.Name()); err != nil {
		return nil, errors.Join(application.ErrInvalidDepthInput, err)
	}
	file, err := os.CreateTemp("", "lanverse-depth-cfr-*.mp4")
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
	fps := min(facts.FPS, 30)
	filter := "setpts=PTS-STARTPTS,scale=w='min(960,iw)':h='min(960,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:flags=bicubic,setsar=1,fps=" + strconv.FormatFloat(fps, 'g', 17, 64)
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-y", "-protocol_whitelist", "file,pipe", "-i", source.File.Name(), "-map", "0:v:0", "-vf", filter, "-fps_mode", "cfr", "-an", "-sn", "-dn", "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-fs", "536870912", file.Name()}
	if _, err := runCommand(stage, p.ffmpeg, args, maxProcessRSS); err != nil {
		return nil, errors.Join(application.ErrInvalidDepthInput, err)
	}
	prepared, err := p.ProbeVideo(stage, file.Name())
	if err != nil {
		return nil, err
	}
	if prepared.FrameCount > 453 || prepared.Width > 960 || prepared.Height > 960 {
		return nil, application.ErrDepthBudgetExceeded
	}
	tolerance := int64(math.Ceil(1000/fps)) + 2
	if prepared.DurationMS < 1 || prepared.FrameCount < 1 || prepared.Width%2 != 0 || prepared.Height%2 != 0 || math.Abs(prepared.FPS-fps) > 0.001 || absMS(prepared.DurationMS-facts.DurationMS) > tolerance || absMS(prepared.DurationMS-int64(math.Round(float64(prepared.FrameCount)/prepared.FPS*1000))) > 2 {
		return nil, application.ErrDepthOutputInvalid
	}
	if err := p.VerifyDecode(stage, file.Name()); err != nil {
		return nil, err
	}
	if err := finishFile(output); err != nil {
		return nil, err
	}
	keep = true
	return &application.PreparedDepthVideo{File: output, Source: facts, Prepared: prepared}, nil
}

// ProbeVideo counts actual decoded frames and prefers the video stream duration.
func (p *Preprocessor) ProbeVideo(ctx context.Context, file string) (application.DepthVideoFacts, error) {
	data, err := runCommand(ctx, p.ffprobe, []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "json", file}, maxProcessRSS)
	if err != nil {
		return application.DepthVideoFacts{}, errors.Join(application.ErrInvalidDepthInput, err)
	}
	var value struct {
		Streams []struct {
			Width, Height int32
		} `json:"streams"`
	}
	if json.Unmarshal(data, &value) != nil || len(value.Streams) != 1 {
		return application.DepthVideoFacts{}, application.ErrInvalidDepthInput
	}
	s := value.Streams[0]
	if s.Width < 1 || s.Height < 1 {
		return application.DepthVideoFacts{}, application.ErrInvalidDepthInput
	}
	frames, duration, err := p.decodedSpan(ctx, file)
	if err != nil {
		return application.DepthVideoFacts{}, err
	}
	return application.DepthVideoFacts{Width: s.Width, Height: s.Height, FrameCount: frames, FPS: float64(frames) / duration, DurationMS: int64(math.Round(duration * 1000))}, nil
}

// decodedSpan counts decoded frames and their timestamp extent. VFR container
// duration can precede the last decoded frame; it is not a complete-video fact.
func (p *Preprocessor) decodedSpan(ctx context.Context, file string) (int, float64, error) {
	var count int
	var first, previous, end float64
	args := []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-select_streams", "v:0", "-show_entries", "frame=best_effort_timestamp_time,duration_time,pkt_duration_time", "-of", "compact=p=0:nk=0", file}
	_, _, err := runNative(ctx, p.ffprobe, args, maxProcessRSS, func(line []byte) error {
		var timestamp, duration string
		for _, field := range strings.Split(string(line), "|") {
			key, value, _ := strings.Cut(field, "=")
			switch key {
			case "best_effort_timestamp_time":
				timestamp = value
			case "duration_time", "pkt_duration_time":
				duration = value
			}
		}
		if timestamp == "" && duration == "" {
			return nil // Empty lines or side-data labels contain no frame facts.
		}
		pts, pe := strconv.ParseFloat(timestamp, 64)
		span, de := strconv.ParseFloat(duration, 64)
		if pe != nil || de != nil || !finite(pts) || !finite(span) || span <= 0 || (count > 0 && pts <= previous) {
			return application.ErrInvalidDepthInput
		}
		if count == 0 {
			first = pts
		}
		if pts+span-first > 15.100001 {
			return application.ErrDepthBudgetExceeded
		}
		count++
		previous = pts
		end = max(end, pts+span-first)
		return nil
	}, true)
	if err != nil {
		return 0, 0, errors.Join(application.ErrInvalidDepthInput, err)
	}
	if count < 1 || !finite(end) || end <= 0 {
		return 0, 0, application.ErrInvalidDepthInput
	}
	return count, end, nil
}

// VerifyDecode reads the entire video stream with decoder failures made fatal.
func (p *Preprocessor) VerifyDecode(ctx context.Context, file string) error {
	_, err := runCommand(ctx, p.ffmpeg, []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe", "-i", file, "-map", "0:v:0", "-an", "-f", "null", "-"}, maxProcessRSS)
	if err != nil {
		return errors.Join(application.ErrDepthOutputInvalid, err)
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func absMS(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func checkDigest(ctx context.Context, file *os.File, size int64, want string) error {
	if _, err := hex.DecodeString(want); err != nil || len(want) != 64 {
		return application.ErrInvalidDepthInput
	}
	h := sha256.New()
	reader := io.NewSectionReader(file, 0, size)
	buffer := make([]byte, 64<<10)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := reader.Read(buffer)
		_, _ = h.Write(buffer[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.Join(application.ErrInvalidDepthInput, err)
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return application.ErrInvalidDepthInput
	}
	return nil
}

func finishFile(file *mediaapp.Downloaded) error {
	info, err := file.File.Stat()
	if err != nil {
		return err
	}
	if info.Size() < 1 || info.Size() >= 500<<20-65536 {
		return application.ErrDepthBudgetExceeded
	}
	file.Size = info.Size()
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(file.File, 0, file.Size)); err != nil {
		return fmt.Errorf("verify native output digest: %w", err)
	}
	file.SHA256 = hex.EncodeToString(h.Sum(nil))
	_, err = file.File.Seek(0, io.SeekStart)
	return err
}
