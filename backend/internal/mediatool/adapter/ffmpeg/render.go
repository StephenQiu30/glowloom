package ffmpeg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/font/opentype"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

const maxExportBytes int64 = 500 << 20
const maxStageBytes int64 = 2 << 30

//go:embed fonts/NotoSansCJKsc-Regular.otf
var fontResource embed.FS

// Renderer owns the immutable licensed typeface. Each render owns its scratch
// directory, command contexts and caption face; callers own the returned file.
type Renderer struct{ typeface *opentype.Font }

// NewRenderer parses the bundled official Noto SC typeface at construction.
func NewRenderer() (*Renderer, error) {
	data, err := fontResource.ReadFile("fonts/NotoSansCJKsc-Regular.otf")
	if err != nil {
		return nil, err
	}
	typeface, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse export typeface: %w", err)
	}
	return &Renderer{typeface: typeface}, nil
}

// Render segments at frame boundaries so 1000 clips never become 1000 open
// inputs. At most the 32 active tracks plus one caption pipe are opened at once.
func (r *Renderer) Render(ctx context.Context, frozen domain.FrozenExport, paths map[uuid.UUID]string, progress func(int, string) error) (*mediaapp.Downloaded, error) {
	if r == nil || r.typeface == nil {
		return nil, application.ErrUnavailable
	}
	parts, err := renderSegments(frozen.Timeline)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "lanverse-export-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	hasAudio := make(map[uuid.UUID]bool)
	for _, part := range parts {
		for _, clip := range part.clips {
			if clip.AssetID == nil {
				return nil, application.ErrInvalidExport
			}
			path, ok := paths[*clip.AssetID]
			if !ok || !filepath.IsAbs(path) {
				return nil, application.ErrInvalidExport
			}
			if _, found := hasAudio[*clip.AssetID]; !found {
				audio, err := sourceHasAudio(ctx, path)
				if err != nil {
					return nil, err
				}
				hasAudio[*clip.AssetID] = audio
			}
		}
	}
	var concat strings.Builder
	var stageBytes int64
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			if err := progress(20+i*60/len(parts), "rendering"); err != nil {
				return nil, err
			}
		}
		name := fmt.Sprintf("segment-%04d.mkv", i)
		if err := r.renderPart(ctx, root, name, frozen.Timeline, part, paths, hasAudio); err != nil {
			return nil, err
		}
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		if err := verifySegment(ctx, filepath.Join(root, name), part.end-part.start, frozen.Timeline.FPS); err != nil {
			return nil, err
		}
		stageBytes += info.Size()
		if stageBytes > maxStageBytes {
			return nil, fmt.Errorf("%w: export scratch budget", application.ErrInvalidExport)
		}
		fmt.Fprintf(&concat, "file '%s'\nduration %s\n", name, frameSeconds(part.end-part.start, frozen.Timeline.FPS))
	}
	if err := os.WriteFile(filepath.Join(root, "concat.txt"), []byte(concat.String()), 0600); err != nil {
		return nil, err
	}
	output, err := os.CreateTemp("", "lanverse-export-final-*.mp4")
	if err != nil {
		return nil, err
	}
	name := output.Name()
	_ = output.Close()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(name)
		}
	}()
	if progress != nil {
		if err := progress(82, "encoding"); err != nil {
			return nil, err
		}
	}
	duration := frameSeconds(parts[len(parts)-1].end, frozen.Timeline.FPS)
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-f", "concat", "-safe", "1", "-i", "concat.txt", "-t", duration, "-c:v", "copy", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-fs", strconv.FormatInt(maxExportBytes+1, 10), name}
	if err := run(ctx, root, args, nil); err != nil {
		return nil, err
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	result := &mediaapp.Downloaded{File: file, MIMEType: "video/mp4"}
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > maxExportBytes {
		return nil, fmt.Errorf("%w: export output budget", application.ErrInvalidExport)
	}
	result.Size = info.Size()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	probe, err := (mediaflow.FFProber{}).Probe(ctx, result)
	width, height := dimensions(frozen.Timeline.AspectRatio)
	expectedMS := parts[len(parts)-1].end * 1000 / int64(frozen.Timeline.FPS)
	if err != nil || probe.Width == nil || probe.Height == nil || int(*probe.Width) != width || int(*probe.Height) != height || probe.DurationMS == nil || abs(int64(*probe.DurationMS)-expectedMS) > int64(1000/frozen.Timeline.FPS)+25 {
		return nil, fmt.Errorf("%w: export output facts", application.ErrInvalidExport)
	}
	keep = true
	return result, nil
}

func (r *Renderer) renderPart(ctx context.Context, root, name string, config canvasdomain.TimelineConfig, part segment, paths map[uuid.UUID]string, audio map[uuid.UUID]bool) error {
	width, height := dimensions(config.AspectRatio)
	duration := frameSeconds(part.end-part.start, config.FPS)
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%d:d=%s", width, height, config.FPS, duration)}
	tracks := make(map[uuid.UUID]canvasdomain.TimelineTrack, len(config.Tracks))
	for _, track := range config.Tracks {
		tracks[track.ID] = track
	}
	filters := []string{"[0:v]setpts=PTS-STARTPTS[v0]", "anullsrc=r=48000:cl=stereo:d=" + duration + "[a0]"}
	visual := "v0"
	audioLabels := []string{"[a0]"}
	for i, clip := range part.clips {
		input := i + 1
		elapsed := max(0, float64(part.start)*1000/float64(config.FPS)-float64(clip.StartMS))
		if clip.Kind == "image" {
			args = append(args, "-loop", "1")
		} else {
			args = append(args, "-ss", seconds(float64(clip.SourceStartMS)+elapsed))
		}
		args = append(args, "-protocol_whitelist", "file,pipe", "-t", duration, "-i", paths[*clip.AssetID])
		if clip.Kind == "image" || clip.Kind == "video" {
			label := fmt.Sprintf("v%d", input)
			crop := ""
			if clip.Crop != nil {
				crop = fmt.Sprintf("crop=%d:%d:%d:%d:exact=1,", clip.Crop.Width, clip.Crop.Height, clip.Crop.X, clip.Crop.Y)
			}
			filters = append(filters, fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS,%sfps=%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1[scaled%d]", input, crop, config.FPS, width, height, width, height, input), fmt.Sprintf("[%s][scaled%d]overlay=eof_action=pass:shortest=0[%s]", visual, input, label))
			visual = label
		}
		if !tracks[clip.TrackID].Muted && clip.Volume > 0 && audio[*clip.AssetID] && clip.Kind != "image" {
			label := fmt.Sprintf("a%d", input)
			filters = append(filters, fmt.Sprintf("[%d:a]atrim=duration=%s,asetpts=PTS-STARTPTS,aresample=48000,aformat=channel_layouts=stereo,%s[%s]", input, duration, audioEnvelope(clip, elapsed), label))
			audioLabels = append(audioLabels, "["+label+"]")
		}
	}
	caption, err := newCaptionReader(ctx, config, r.typeface, width, height, part.start, part.end-part.start)
	if err != nil {
		return err
	}
	defer func() { _ = caption.face.Close() }()
	captionIndex := len(part.clips) + 1
	args = append(args, "-f", "image2pipe", "-vcodec", "png", "-r", strconv.Itoa(config.FPS), "-i", "pipe:0")
	filters = append(filters, fmt.Sprintf("[%d:v]format=rgba[caption]", captionIndex), fmt.Sprintf("[%s][caption]overlay=eof_action=pass:shortest=0[vout]", visual), strings.Join(audioLabels, "")+fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=0[aout]", len(audioLabels)))
	args = append(args, "-filter_complex_threads", "1", "-filter_complex", strings.Join(filters, ";"), "-map", "[vout]", "-map", "[aout]", "-t", duration, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", "-fs", strconv.FormatInt(maxStageBytes+1, 10), name)
	return run(ctx, root, args, caption)
}

func run(ctx context.Context, root string, args []string, stdin io.Reader) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Dir = root
	cmd.Stdin = stdin
	cmd.Stderr = &diagnostics{}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("local media render: %w", err)
	}
	return nil
}

func sourceHasAudio(ctx context.Context, path string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "stream=codec_type", "-of", "json", path)
	var out diagnostics
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return false, err
	}
	var result struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil {
		return false, application.ErrInvalidExport
	}
	for _, stream := range result.Streams {
		if stream.CodecType == "audio" {
			return true, nil
		}
	}
	return false, nil
}

type diagnostics struct{ bytes.Buffer }

func (w *diagnostics) Write(data []byte) (int, error) {
	if w.Len() < 4096 {
		_, _ = w.Buffer.Write(data[:min(len(data), 4096-w.Len())])
	}
	return len(data), nil
}
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func verifySegment(ctx context.Context, file string, frames int64, fps int) error {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "format=duration", "-of", "json", file)
	output := &diagnostics{}
	cmd.Stdout = output
	cmd.Stderr = &diagnostics{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("verify export segment: %w", err)
	}
	var parsed struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(output.Bytes(), &parsed) != nil {
		return application.ErrInvalidExport
	}
	seconds, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	expected := float64(frames) / float64(fps)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || math.Abs(seconds-expected) > 1/float64(fps)+0.025 {
		return fmt.Errorf("%w: truncated export segment", application.ErrInvalidExport)
	}
	return nil
}
