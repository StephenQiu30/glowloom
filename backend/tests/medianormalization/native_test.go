package medianormalization_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/ffmpeg"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
)

func nativeWebM(t *testing.T, codec, duration string, extra ...string) []byte {
	t.Helper()
	name := filepath.Join(t.TempDir(), "source.webm")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10", "-t", duration,
		"-c:v", codec, "-threads", "1", "-deadline", "realtime", "-cpu-used", "8", "-an", "-f", "webm", "-live", "1"}
	args = append(args, extra...)
	args = append(args, name)
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("create native WebM fixture: %v: %s", err, output)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func nativeSource(t *testing.T, data []byte) *application.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "source-*.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	source := &application.File{File: file, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), MIMEType: "video/webm"}
	t.Cleanup(func() { _ = source.Close() })
	return source
}

func nativeTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	return dir
}

func assertNativeTemps(t *testing.T, dir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != want {
		t.Fatalf("temporary files=%v, want %d", entries, want)
	}
}

func TestNativeNormalizerRealDurationlessRecordings(t *testing.T) {
	for _, codec := range []string{"libvpx", "libvpx-vp9"} {
		t.Run(codec, func(t *testing.T) {
			source := nativeSource(t, nativeWebM(t, codec, "0.6"))
			sourceName := source.File.Name()
			dir := nativeTempDir(t)
			result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = result.File.Close() })
			if result.File == nil || result.File.File == nil || result.File.File == source.File || result.File.MIMEType != "video/mp4" {
				t.Fatalf("canonical result=%+v", result)
			}
			wantCodec := map[string]string{"libvpx": "vp8", "libvpx-vp9": "vp9"}[codec]
			if result.SourceCodec != wantCodec {
				t.Fatalf("source codec=%q, want %q", result.SourceCodec, wantCodec)
			}
			// No second full-sized source/output copy is left in the temporary directory.
			assertNativeTemps(t, dir, 1)
			data, err := io.ReadAll(result.File.File)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if result.File.Size != int64(len(data)) || result.File.SHA256 != hex.EncodeToString(hash[:]) || result.File.SHA256 == source.SHA256 {
				t.Fatalf("canonical receipt does not match actual output: %+v", result.File)
			}
			output, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,width,height,avg_frame_rate", "-of", "json", result.File.File.Name()).Output()
			if err != nil {
				t.Fatal(err)
			}
			var probe struct {
				Format struct {
					Duration string `json:"duration"`
					Name     string `json:"format_name"`
				} `json:"format"`
				Streams []struct {
					Type   string `json:"codec_type"`
					Codec  string `json:"codec_name"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"streams"`
			}
			if err := json.Unmarshal(output, &probe); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(probe.Format.Name, "mp4") || probe.Format.Duration != "0.600000" || len(probe.Streams) != 1 || probe.Streams[0].Type != "video" || probe.Streams[0].Codec != "h264" || probe.Streams[0].Width != 64 || probe.Streams[0].Height != 48 {
				t.Fatalf("actual canonical probe=%s", output)
			}
			if source.File == nil {
				t.Fatal("normalization closed the caller's source")
			}
			if _, err := os.Stat(sourceName); err != nil {
				t.Fatalf("normalization removed the caller's source: %v", err)
			}
			if err := result.File.Close(); err != nil {
				t.Fatal(err)
			}
			if err := result.File.Close(); err != nil {
				t.Fatalf("second close: %v", err)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
}

func TestNativeNormalizerValidatesActualSourceReceipt(t *testing.T) {
	data := nativeWebM(t, "libvpx", "0.3")
	for _, tc := range []struct {
		name   string
		change func(*application.File)
		want   error
	}{
		{"declared size", func(f *application.File) { f.Size++ }, application.ErrInvalidInput},
		{"declared hash", func(f *application.File) { f.SHA256 = strings.Repeat("0", 64) }, application.ErrInvalidInput},
		{"declared too large", func(f *application.File) { f.Size = application.MaxBytes + 1 }, application.ErrTooLarge},
		{"actual too large", func(f *application.File) {
			if err := f.File.Truncate(application.MaxBytes + 1); err != nil {
				t.Fatal(err)
			}
		}, application.ErrTooLarge},
		{"declared MIME", func(f *application.File) { f.MIMEType = "video/mp4" }, application.ErrUnsupported},
		{"actual magic", func(f *application.File) {
			if _, err := f.File.WriteAt(bytes.Repeat([]byte{'x'}, 64), 0); err != nil {
				t.Fatal(err)
			}
		}, application.ErrUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := nativeSource(t, data)
			tc.change(source)
			dir := nativeTempDir(t)
			result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
			if !errors.Is(err, tc.want) || result.File != nil {
				t.Fatalf("result=%+v error=%v, want %v", result, err, tc.want)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
}

func nativeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

const nativeSourceProbe = `{"format":{"format_name":"matroska,webm"},"streams":[{"codec_type":"video","codec_name":"vp9","width":64,"height":48,"avg_frame_rate":"10/1"}]}`
const nativeCanonicalProbe = `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"0.6"},"streams":[{"codec_type":"video","codec_name":"h264","width":64,"height":48,"avg_frame_rate":"10/1","duration":"0.6"}]}`

// These process fixtures let boundary tests distinguish actual ffprobe facts
// from requested encoder flags without depending on an encoder implementation.
func nativeProcesses(t *testing.T, sourceProbe, canonicalProbe, encoder string) {
	t.Helper()
	dir := t.TempDir()
	nativeScript(t, dir, "ffprobe", fmt.Sprintf(`case "$*" in
*glowloom-normalized-*) printf '%%s' '%s' ;;
*) printf '%%s' '%s' ;;
esac`, canonicalProbe, sourceProbe))
	if encoder != "" {
		nativeScript(t, dir, "ffmpeg", encoder)
	}
	t.Setenv("PATH", dir)
}

const nativeEncoder = `printf '\000\000\000\030ftypmp42\000\000\000\000mp42isom'
printf 'out_time_us=600000\nprogress=end\n' >&2`

func TestNativeNormalizerEnforcesSourceProbeLimits(t *testing.T) {
	data := nativeWebM(t, "libvpx-vp9", "0.3")
	for _, tc := range []struct{ name, probe string }{
		{"non-WebM container", strings.Replace(nativeSourceProbe, "matroska,webm", "matroska", 1)},
		{"wrong codec", strings.Replace(nativeSourceProbe, "vp9", "h264", 1)},
		{"audio track", strings.Replace(nativeSourceProbe, `}]}`, `},{"codec_type":"audio","codec_name":"opus"}]}`, 1)},
		{"edge limit", strings.Replace(nativeSourceProbe, `"width":64`, `"width":8193`, 1)},
		{"pixel limit", strings.ReplaceAll(strings.Replace(nativeSourceProbe, `"width":64`, `"width":8192`, 1), `"height":48`, `"height":8192`)},
		{"FPS limit", strings.Replace(nativeSourceProbe, "10/1", "61/1", 1)},
		{"extreme FPS", strings.Replace(nativeSourceProbe, "10/1", "1001/1", 1)},
		{"nonfinite FPS", strings.Replace(nativeSourceProbe, "10/1", "NaN/1", 1)},
		{"overflow FPS", strings.Replace(nativeSourceProbe, "10/1", "1e308/1e-308", 1)},
		{"negative FPS", strings.Replace(nativeSourceProbe, "10/1", "-1/1", 1)},
		{"format duration", strings.Replace(nativeSourceProbe, `"format_name":"matroska,webm"`, `"format_name":"matroska,webm","duration":"60.00001"`, 1)},
		{"stream duration", strings.Replace(nativeSourceProbe, `"avg_frame_rate":"10/1"`, `"avg_frame_rate":"10/1","duration":"60.1"`, 1)},
		{"nonfinite duration", strings.Replace(nativeSourceProbe, `"format_name":"matroska,webm"`, `"format_name":"matroska,webm","duration":"NaN"`, 1)},
		{"bounded probe JSON", strings.Repeat("x", 1_000_001)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := nativeSource(t, data)
			dir := nativeTempDir(t)
			nativeProcesses(t, tc.probe, nativeCanonicalProbe, nativeEncoder)
			result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
			if !errors.Is(err, application.ErrUnsupported) || result.File != nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
}

func TestNativeNormalizerVerifiesCanonicalOutput(t *testing.T) {
	data := nativeWebM(t, "libvpx-vp9", "0.3")
	for _, tc := range []struct{ name, probe, encoder string }{
		{"wrong actual codec", strings.Replace(nativeCanonicalProbe, "h264", "vp9", 1), nativeEncoder},
		{"wrong actual container", strings.Replace(nativeCanonicalProbe, "mov,mp4,m4a,3gp,3g2,mj2", "matroska,webm", 1), nativeEncoder},
		{"long actual duration", strings.ReplaceAll(nativeCanonicalProbe, `"duration":"0.6"`, `"duration":"60.1"`), nativeEncoder},
		{"missing actual duration", strings.ReplaceAll(nativeCanonicalProbe, `,"duration":"0.6"`, ""), nativeEncoder},
		{"oversized actual dimensions", strings.Replace(nativeCanonicalProbe, `"width":64`, `"width":8193`, 1), nativeEncoder},
		{"unexpected audio", strings.Replace(nativeCanonicalProbe, `}]}`, `},{"codec_type":"audio","codec_name":"aac"}]}`, 1), nativeEncoder},
		{"actual FPS over limit", strings.Replace(nativeCanonicalProbe, "10/1", "61/1", 1), nativeEncoder},
		{"nonfinite actual FPS", strings.Replace(nativeCanonicalProbe, "10/1", "NaN/1", 1), nativeEncoder},
		{"false MP4 magic", nativeCanonicalProbe, `printf 'not an MP4'; printf 'progress=end\n' >&2`},
		{"empty output", nativeCanonicalProbe, `printf 'progress=end\n' >&2`},
		{"missing completion", nativeCanonicalProbe, strings.Replace(nativeEncoder, `progress=end\n`, "", 1)},
		{"decoded duration over limit", nativeCanonicalProbe, strings.Replace(nativeEncoder, "600000", "60000001", 1)},
		{"unbounded progress line", nativeCanonicalProbe, nativeEncoder + "\nprintf '%s' '" + strings.Repeat("x", 4097) + "' >&2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := nativeSource(t, data)
			dir := nativeTempDir(t)
			nativeProcesses(t, nativeSourceProbe, tc.probe, tc.encoder)
			result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
			if !errors.Is(err, application.ErrUnsupported) || result.File != nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
}

func TestNativeNormalizerPreservesInfrastructureAndCancellationErrors(t *testing.T) {
	data := nativeWebM(t, "libvpx-vp9", "0.3")
	for _, name := range []string{"probe missing", "encoder missing", "probe signal", "encoder signal", "probe cancelled", "encoder cancelled"} {
		t.Run(name, func(t *testing.T) {
			source := nativeSource(t, data)
			dir := nativeTempDir(t)
			bin := t.TempDir()
			nativeScript(t, bin, "ffprobe", fmt.Sprintf("printf '%%s' '%s'", nativeSourceProbe))
			nativeScript(t, bin, "ffmpeg", nativeEncoder)
			ctx := t.Context()
			switch name {
			case "probe missing":
				if err := os.Remove(filepath.Join(bin, "ffprobe")); err != nil {
					t.Fatal(err)
				}
			case "encoder missing":
				if err := os.Remove(filepath.Join(bin, "ffmpeg")); err != nil {
					t.Fatal(err)
				}
			case "probe signal":
				nativeScript(t, bin, "ffprobe", "kill -TERM $$")
			case "encoder signal":
				nativeScript(t, bin, "ffmpeg", "kill -TERM $$")
			case "probe cancelled", "encoder cancelled":
				command := "ffprobe"
				if name == "encoder cancelled" {
					command = "ffmpeg"
				}
				nativeScript(t, bin, command, "exec /bin/sleep 10")
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			t.Setenv("PATH", bin)
			result, err := (ffmpeg.Normalizer{}).Normalize(ctx, source)
			if err == nil || errors.Is(err, application.ErrUnsupported) || errors.Is(err, application.ErrInvalidInput) || result.File != nil {
				t.Fatalf("infrastructure error misclassified: result=%+v error=%v", result, err)
			}
			if strings.HasSuffix(name, "cancelled") && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancellation not preserved: %v", err)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := (ffmpeg.Normalizer{}).Normalize(ctx, nil); !errors.Is(err, context.Canceled) || result.File != nil {
		t.Fatalf("pre-cancelled call result=%+v error=%v", result, err)
	}
}

func TestNativeNormalizerBoundsActualEncoderOutput(t *testing.T) {
	source := nativeSource(t, nativeWebM(t, "libvpx-vp9", "0.3"))
	dir := nativeTempDir(t)
	// Stream beyond the hard cap instead of trusting a claimed output size.
	// exec ensures cancellation owns the only encoder process, including pipes.
	nativeProcesses(t, nativeSourceProbe, nativeCanonicalProbe, "exec /bin/dd if=/dev/zero bs=1048576 count=501 2>/dev/null")
	result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
	if !errors.Is(err, application.ErrTooLarge) || result.File != nil {
		t.Fatalf("oversized actual output result=%+v error=%v", result, err)
	}
	assertNativeTemps(t, dir, 0)
}

func TestNativeFileCloseOwnsOnlyItsTemporaryFile(t *testing.T) {
	var absent *application.File
	if err := absent.Close(); err != nil {
		t.Fatal(err)
	}
	file := nativeSource(t, []byte("temporary file"))
	name := file.File.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if file.File != nil {
		t.Fatal("Close retained a closed file handle")
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned file not removed: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func TestNativeNormalizerRejectsRealLongAndCorruptRecordings(t *testing.T) {
	short := nativeWebM(t, "libvpx-vp9", "0.3")
	long := nativeWebM(t, "libvpx-vp9", "60.2")
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"long durationless recording", long},
		{"truncated container", short[:80]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := nativeSource(t, tc.data)
			dir := nativeTempDir(t)
			result, err := (ffmpeg.Normalizer{}).Normalize(t.Context(), source)
			if !errors.Is(err, application.ErrUnsupported) || result.File != nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			assertNativeTemps(t, dir, 0)
		})
	}
}
