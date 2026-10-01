package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

// FFUploadNormalizer fully decodes one silent VP8/VP9 WebM recording to H264
// MP4. It never changes the caller's original or silently truncates a recording.
type FFUploadNormalizer struct{}

// Normalize returns independently hashed canonical bytes. A durationless live
// WebM is accepted only after the entire decoder and encoder actually finish.
func (FFUploadNormalizer) Normalize(ctx context.Context, source *application.Downloaded) (application.NormalizedUpload, error) {
	if err := ctx.Err(); err != nil {
		return application.NormalizedUpload{}, err
	}
	if source == nil || source.File == nil || source.Size < 1 || source.MIMEType != "video/webm" {
		return application.NormalizedUpload{}, application.ErrUnsupportedUpload
	}
	if source.Size > application.MaxUploadVideoBytes {
		return application.NormalizedUpload{}, application.ErrUploadTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := verifyNormalizationSource(ctx, source); err != nil {
		return application.NormalizedUpload{}, err
	}
	codec, err := probeWebMRecording(ctx, source)
	if err != nil {
		return application.NormalizedUpload{}, err
	}
	output, err := os.CreateTemp("", "lanverse-normalized-*-.mp4")
	if err != nil {
		return application.NormalizedUpload{}, fmt.Errorf("create canonical recording: %w", err)
	}
	file := &application.Downloaded{File: output, MIMEType: "video/mp4"}
	keep := false
	defer func() {
		if !keep {
			_ = file.Close()
		}
	}()
	processCtx, stopProcess := context.WithCancel(ctx)
	defer stopProcess()
	writer := normalizationOutput{file: output, cancel: stopProcess}
	progress := normalizationProgress{cancel: stopProcess}
	// Fragmented MP4 allows a bounded writer on stdout; no -t/-fs shortcut is
	// used because either can produce a valid-looking but incomplete asset.
	cmd := exec.CommandContext(processCtx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-err_detect", "explode",
		"-protocol_whitelist", "file,pipe", "-threads", "2", "-fflags", "+genpts", "-i", source.File.Name(),
		"-map", "0:v:0", "-an", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1",
		"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2,setsar=1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", "-bf", "0", "-threads", "2",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof", "-brand", "mp42", "-progress", "pipe:2", "-f", "mp4", "pipe:1")
	cmd.Stdout, cmd.Stderr = &writer, &progress
	err = cmd.Run()
	if writer.overflow {
		return application.NormalizedUpload{}, application.ErrUploadTooLarge
	}
	if progress.tooLong || progress.invalid {
		return application.NormalizedUpload{}, application.ErrUnsupportedUpload
	}
	if err != nil {
		if ctx.Err() != nil {
			return application.NormalizedUpload{}, ctx.Err()
		}
		mapped := mediaProcessError(ctx, err)
		if errors.Is(mapped, ErrUnsupportedMedia) {
			mapped = application.ErrUnsupportedUpload
		}
		return application.NormalizedUpload{}, fmt.Errorf("normalize recording: %w", mapped)
	}
	if !progress.completed || writer.size < 1 {
		return application.NormalizedUpload{}, application.ErrUnsupportedUpload
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return application.NormalizedUpload{}, fmt.Errorf("rewind canonical recording: %w", err)
	}
	actual, err := application.ReadUpload(ctx, output, "canonical.mp4")
	if err != nil {
		return application.NormalizedUpload{}, err
	}
	defer func() { _ = actual.Close() }()
	file.Size, file.SHA256 = actual.Size, actual.SHA256
	probe, err := (FFUploadProber{}).Probe(ctx, file)
	if err != nil {
		return application.NormalizedUpload{}, err
	}
	if probe.Extension != "mp4" || probe.Codec == nil || *probe.Codec != "h264" || probe.DurationMS == nil || *probe.DurationMS < 1 || *probe.DurationMS > 60_000 ||
		probe.Width == nil || probe.Height == nil || *probe.Width < 2 || *probe.Height < 2 || *probe.Width > 8192 || *probe.Height > 8192 ||
		int64(*probe.Width)*int64(*probe.Height) > 40_000_000 {
		return application.NormalizedUpload{}, application.ErrUnsupportedUpload
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return application.NormalizedUpload{}, fmt.Errorf("rewind canonical recording: %w", err)
	}
	keep = true
	return application.NormalizedUpload{File: file, SourceCodec: codec}, nil
}

func verifyNormalizationSource(ctx context.Context, source *application.Downloaded) error {
	info, err := source.File.Stat()
	if err != nil || info.Size() != source.Size {
		return application.ErrInvalidUpload
	}
	var magic [512]byte
	n, err := source.File.ReadAt(magic[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read recording header: %w", err)
	}
	// DetectContentType checks the EBML DocType, distinguishing WebM from
	// Matroska instead of trusting the filename or multipart Content-Type.
	if http.DetectContentType(magic[:n]) != "video/webm" {
		return application.ErrUnsupportedUpload
	}
	if _, err := source.File.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind recording: %w", err)
	}
	hash := sha256.New()
	var buf [64 * 1024]byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := source.File.Read(buf[:])
		_, _ = hash.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("hash recording: %w", err)
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != source.SHA256 {
		return application.ErrInvalidUpload
	}
	return nil
}

func probeWebMRecording(ctx context.Context, source *application.Downloaded) (string, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries",
		"format=duration,format_name:stream=codec_type,codec_name,width,height,avg_frame_rate,duration", "-of", "json", source.File.Name())
	var output probeOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(mediaProcessError(ctx, err), ErrUnsupportedMedia) {
			return "", application.ErrUnsupportedUpload
		}
		return "", fmt.Errorf("probe recording: %w", err)
	}
	var parsed struct {
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type     string `json:"codec_type"`
			Codec    string `json:"codec_name"`
			Width    int32  `json:"width"`
			Height   int32  `json:"height"`
			FPS      string `json:"avg_frame_rate"`
			Duration string `json:"duration"`
		} `json:"streams"`
	}
	if output.overflow || json.Unmarshal(output.Bytes(), &parsed) != nil || !strings.Contains(","+parsed.Format.Name+",", ",webm,") || len(parsed.Streams) != 1 {
		return "", application.ErrUnsupportedUpload
	}
	stream := parsed.Streams[0]
	if stream.Type != "video" || (stream.Codec != "vp8" && stream.Codec != "vp9") || stream.Width < 2 || stream.Height < 2 ||
		stream.Width > 8192 || stream.Height > 8192 || int64(stream.Width)*int64(stream.Height) > 40_000_000 {
		return "", application.ErrUnsupportedUpload
	}
	if fps, ok := frameRate(stream.FPS); ok && fps > 60 {
		return "", application.ErrUnsupportedUpload
	}
	for _, raw := range []string{parsed.Format.Duration, stream.Duration} {
		if excessiveUploadDuration(raw) {
			return "", application.ErrUnsupportedUpload
		}
		if duration, ok := uploadMilliseconds(raw); ok && duration > 60_000 {
			return "", application.ErrUnsupportedUpload
		}
	}
	return stream.Codec, nil
}

type normalizationOutput struct {
	file     *os.File
	cancel   context.CancelFunc
	size     int64
	overflow bool
}

func (w *normalizationOutput) Write(value []byte) (int, error) {
	remaining := application.MaxUploadVideoBytes - w.size
	if int64(len(value)) > remaining {
		w.overflow = true
		w.cancel()
		return 0, application.ErrUploadTooLarge
	}
	n, err := w.file.Write(value)
	w.size += int64(n)
	return n, err
}

type normalizationProgress struct {
	cancel    context.CancelFunc
	pending   []byte
	tooLong   bool
	invalid   bool
	completed bool
}

func (p *normalizationProgress) Write(value []byte) (int, error) {
	p.pending = append(p.pending, value...)
	for {
		line, rest, found := bytes.Cut(p.pending, []byte{'\n'})
		if !found {
			break
		}
		p.pending = rest
		if len(line) > 4096 {
			p.invalid = true
			p.cancel()
			continue
		}
		if raw, ok := strings.CutPrefix(string(line), "out_time_us="); ok && raw != "N/A" {
			micros, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				p.invalid = true
				p.cancel()
			} else if micros > 60_000_000 {
				p.tooLong = true
				p.cancel()
			}
		}
		if string(line) == "progress=end" {
			p.completed = true
		}
	}
	if len(p.pending) > 4096 {
		p.invalid = true
		p.pending = nil
		p.cancel()
	}
	return len(value), nil
}
