// Package ffmpeg implements bounded, file-only recording normalization.
package ffmpeg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
)

// Normalizer fully decodes a single silent VP8/VP9 WebM stream to H264 MP4.
// It borrows the source, never closes it, and transfers ownership of the result.
type Normalizer struct{}

// Normalize validates actual input bytes and independently probes actual output.
// A durationless live WebM is accepted only after the full decoder and encoder
// finish; neither a duration nor a file-size truncation flag is used.
func (Normalizer) Normalize(ctx context.Context, source *application.File) (application.Result, error) {
	if err := ctx.Err(); err != nil {
		return application.Result{}, err
	}
	if source == nil || source.File == nil || source.Size < 1 || source.MIMEType != "video/webm" {
		return application.Result{}, application.ErrUnsupported
	}
	if source.Size > application.MaxBytes {
		return application.Result{}, application.ErrTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := verifySource(ctx, source); err != nil {
		return application.Result{}, err
	}
	codec, err := probeSource(ctx, source.File.Name())
	if err != nil {
		return application.Result{}, err
	}
	output, err := os.CreateTemp("", "glowloom-normalized-*-.mp4")
	if err != nil {
		return application.Result{}, fmt.Errorf("create canonical recording: %w", err)
	}
	file := &application.File{File: output, MIMEType: "video/mp4"}
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
	// Fragmented MP4 supports a bounded stdout writer without -t/-fs, which
	// could otherwise produce valid-looking but silently incomplete assets.
	cmd := exec.CommandContext(processCtx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-err_detect", "explode",
		"-protocol_whitelist", "file,pipe", "-threads", "2", "-fflags", "+genpts", "-i", source.File.Name(),
		"-map", "0:v:0", "-an", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1",
		"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2,setsar=1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p", "-bf", "0", "-threads", "2",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof", "-brand", "mp42", "-progress", "pipe:2", "-f", "mp4", "pipe:1")
	cmd.Stdout, cmd.Stderr = &writer, &progress
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return application.Result{}, ctx.Err()
	}
	if writer.overflow {
		return application.Result{}, application.ErrTooLarge
	}
	if writer.err != nil {
		return application.Result{}, fmt.Errorf("write canonical recording: %w", writer.err)
	}
	if progress.tooLong || progress.invalid {
		return application.Result{}, application.ErrUnsupported
	}
	if err != nil {
		return application.Result{}, fmt.Errorf("normalize recording: %w", mediaProcessError(ctx, err))
	}
	if !progress.completed || writer.size < 1 {
		return application.Result{}, application.ErrUnsupported
	}
	magic, err := fileMagic(output)
	if err != nil {
		return application.Result{}, err
	}
	if !canonicalMagic(magic) {
		return application.Result{}, application.ErrUnsupported
	}
	// Hash the actual file rather than trusting encoder settings or only the
	// bytes offered to its writer. No additional full-size temporary copy exists.
	size, digest, err := fileReceipt(ctx, output)
	if err != nil {
		return application.Result{}, err
	}
	if size != writer.size {
		return application.Result{}, application.ErrInvalidInput
	}
	file.Size, file.SHA256 = size, digest
	if err := probeCanonical(ctx, output.Name()); err != nil {
		return application.Result{}, err
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return application.Result{}, fmt.Errorf("rewind canonical recording: %w", err)
	}
	keep = true
	return application.Result{File: file, SourceCodec: codec}, nil
}

func verifySource(ctx context.Context, source *application.File) error {
	info, err := source.File.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return application.ErrInvalidInput
	}
	if info.Size() > application.MaxBytes {
		return application.ErrTooLarge
	}
	if info.Size() != source.Size {
		return application.ErrInvalidInput
	}
	magic, err := fileMagic(source.File)
	if err != nil {
		return err
	}
	// DetectContentType checks EBML DocType, distinguishing real WebM from
	// Matroska without trusting the name or the declared MIME type.
	if http.DetectContentType(magic) != "video/webm" {
		return application.ErrUnsupported
	}
	size, digest, err := fileReceipt(ctx, source.File)
	if err != nil {
		return err
	}
	if size != source.Size || digest != source.SHA256 {
		return application.ErrInvalidInput
	}
	return nil
}

func fileMagic(file *os.File) ([]byte, error) {
	var magic [512]byte
	n, err := file.ReadAt(magic[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read recording header: %w", err)
	}
	return magic[:n], nil
}

func canonicalMagic(magic []byte) bool {
	if len(magic) < 12 || string(magic[4:8]) != "ftyp" {
		return false
	}
	switch string(magic[8:12]) {
	case "isom", "iso2", "mp41", "mp42", "avc1", "M4V ":
		return true
	default:
		return false
	}
}

// fileReceipt uses bounded positional reads, preserving the source's cursor.
func fileReceipt(ctx context.Context, file *os.File) (int64, string, error) {
	reader := io.NewSectionReader(file, 0, application.MaxBytes+1)
	hash := sha256.New()
	var size int64
	var buf [64 * 1024]byte
	for {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		n, err := reader.Read(buf[:])
		size += int64(n)
		if size > application.MaxBytes {
			return 0, "", application.ErrTooLarge
		}
		_, _ = hash.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			return size, hex.EncodeToString(hash.Sum(nil)), nil
		}
		if err != nil {
			return 0, "", fmt.Errorf("hash recording: %w", err)
		}
	}
}

type normalizationOutput struct {
	file     *os.File
	cancel   context.CancelFunc
	size     int64
	overflow bool
	err      error
}

func (w *normalizationOutput) Write(value []byte) (int, error) {
	if int64(len(value)) > application.MaxBytes-w.size {
		w.overflow = true
		w.cancel()
		return 0, application.ErrTooLarge
	}
	n, err := w.file.Write(value)
	w.size += int64(n)
	if err != nil {
		w.err = err
		w.cancel()
	}
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
