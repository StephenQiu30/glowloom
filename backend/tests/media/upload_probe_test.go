package media_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestUploadProbeAcceptsREQ30Containers(t *testing.T) {
	for _, tc := range []struct {
		name, mime, kind string
		args             []string
	}{
		{"reference.mov", "video/quicktime", "video", []string{"-f", "lavfi", "-i", "color=size=64x64:rate=10", "-t", "0.2", "-c:v", "libx264", "-pix_fmt", "yuv420p"}},
		{"reference.m4a", "audio/mp4", "audio", []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "aac"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join(t.TempDir(), tc.name)
			args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, tc.args...)
			if output, err := exec.CommandContext(t.Context(), "ffmpeg", append(args, fixture)...).CombinedOutput(); err != nil {
				t.Fatalf("create upload fixture: %v: %s", err, output)
			}
			file, err := os.Open(fixture)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			probe, err := (mediaflow.FFProber{}).Probe(t.Context(), &mediaapp.Downloaded{File: file, MIMEType: tc.mime})
			if err != nil || probe.Kind != domain.Kind(tc.kind) || probe.DurationMS == nil {
				t.Fatalf("REQ30 %s probe = %+v, %v", tc.name, probe, err)
			}
		})
	}
}

func TestUploadProbeDoesNotIgnoreDurationOverflowOrRoundPastLimit(t *testing.T) {
	for _, tc := range []struct {
		duration  string
		wantMS    int32
		wantError bool
	}{
		{"60.000001", 60001, false},
		{"100000000", 0, true},
	} {
		t.Run(tc.duration, func(t *testing.T) {
			bin := t.TempDir()
			body := fmt.Sprintf(`{"format":{"duration":"%s","format_name":"mov,mp4"},"streams":[{"codec_type":"video","codec_name":"h264","width":64,"height":64,"avg_frame_rate":"1/1","duration":"10"}]}`, tc.duration)
			if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte("#!/bin/sh\nprintf '%s' '"+body+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			file, err := os.CreateTemp(t.TempDir(), "probe.mp4")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			result, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), &mediaapp.Downloaded{File: file, MIMEType: "video/mp4"})
			if tc.wantError {
				if !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
					t.Fatalf("duration overflow accepted: %+v err=%v", result, err)
				}
				return
			}
			if err != nil || result.DurationMS == nil || *result.DurationMS != tc.wantMS {
				t.Fatalf("fractional boundary duration=%+v err=%v", result, err)
			}
		})
	}
}

func TestUploadRejectsOversizedFFprobeDimensionsBeforeRenderer(t *testing.T) {
	bin := t.TempDir()
	body := `{"format":{"format_name":"png_pipe"},"streams":[{"codec_type":"video","codec_name":"png","width":8193,"height":1}]}`
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte("#!/bin/sh\nprintf '%s' '"+body+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	in := uploadInput(t)
	generated, err := (mediaflow.FFProber{}).Probe(t.Context(), in.File)
	if err != nil || generated.Width == nil || *generated.Width != 8193 {
		t.Fatalf("generated probe semantics changed: %+v err=%v", generated, err)
	}
	renderer := &uploadRenderSpy{}
	service := mediaapp.NewUploadService(&uploadRepoFake{}, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, renderer, &uploadObjectsFake{}, time.Now)
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUnsupportedUpload) || renderer.calls != 0 {
		t.Fatalf("untrusted oversized probe facts reached renderer: err=%v calls=%d", err, renderer.calls)
	}
}

func TestUploadProbeUsesLongestContainerOrAVTrackDuration(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "long-audio.mp4")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=size=64x48:rate=1:duration=10", "-f", "lavfi", "-i", "sine=frequency=440:duration=90",
		"-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create dual-track fixture: %v: %s", err, output)
	}
	reader, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	file, err := mediaapp.ReadUpload(t.Context(), reader, "reference.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	generated, err := (mediaflow.FFProber{}).Probe(t.Context(), file)
	if err != nil || generated.DurationMS == nil || *generated.DurationMS != 10000 {
		t.Fatalf("generated duration semantics changed: %+v err=%v", generated, err)
	}
	upload, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), file)
	if err != nil || upload.DurationMS == nil || *upload.DurationMS < 90000 {
		t.Fatalf("upload ignored longer audio/container duration: %+v err=%v", upload, err)
	}
	repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
	service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.File = file
	in.Request.FileName = "reference.mp4"
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUnsupportedUpload) || repo.commits != 0 || len(objects.items) != 0 {
		t.Fatalf("90-second mixed-track upload became referenceable: %v", err)
	}
}
