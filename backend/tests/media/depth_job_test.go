package media_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func depthJobFrozen() tooldomain.FrozenDepth {
	w, h, d := int32(64), int32(64), int32(2000)
	return tooldomain.FrozenDepth{ProfileID: tooldomain.DepthProfileID, Input: tooldomain.FrozenSource{AssetID: uuid.New(), Revision: 1, Kind: "video", ObjectKey: "projects/private/video/source.mp4", MIMEType: "video/mp4", ByteSize: 1024, SHA256: strings.Repeat("a", 64), Width: &w, Height: &h, DurationMS: &d}}
}
func depthJobOutput(f tooldomain.FrozenDepth) *toolapp.DepthOutput {
	w, h, d, fps, codec := int32(1920), int32(1080), int32(2000), float64(30), "h264"
	facts := toolapp.DepthVideoFacts{Width: 64, Height: 64, FrameCount: 60, FPS: 30, DurationMS: 2000}
	out := facts
	out.Width, out.Height = w, h
	return &toolapp.DepthOutput{File: &mediaapp.Downloaded{Size: 2048, MIMEType: "video/mp4", SHA256: strings.Repeat("b", 64)}, Probe: mediaapp.ProbeResult{Kind: mediadomain.KindVideo, Extension: "mp4", Width: &w, Height: &h, DurationMS: &d, FPS: &fps, Codec: &codec}, Receipt: toolapp.DepthReceipt{ProfileID: tooldomain.DepthProfileID, VDACommit: tooldomain.DepthVDACommit, ModelSHA256: tooldomain.DepthModelSHA256, SourceTreeSHA256: tooldomain.DepthSourceTreeSHA256, RuntimeVersion: "3.11/2.8", Device: "mps", InputSHA256: f.Input.SHA256, Source: facts, Prepared: facts, Output: out, Native: toolapp.DepthExecutionFacts{ElapsedMS: 1000, PeakRSSBytes: 1024 * 1024, RSSLimitBytes: 8 << 30, ProcessGroupJoined: true}}}
}
func TestDepthJobClosedSourceBudgetAndNativeReceipt(t *testing.T) {
	f := depthJobFrozen()
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := toolapp.ValidateDepthReceipt(f, depthJobOutput(f)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*toolapp.DepthOutput){
		func(o *toolapp.DepthOutput) { o.Receipt.ProfileID = "arbitrary-profile" },
		func(o *toolapp.DepthOutput) { o.Receipt.InputSHA256 = strings.Repeat("c", 64) },
		func(o *toolapp.DepthOutput) { o.Receipt.ModelSHA256 = strings.Repeat("c", 64) },
		func(o *toolapp.DepthOutput) { o.Receipt.Native.ProcessGroupJoined = false },
		func(o *toolapp.DepthOutput) { o.Receipt.Output.FrameCount = 59 },
		func(o *toolapp.DepthOutput) { o.Receipt.Output.DurationMS = 1000 },
		func(o *toolapp.DepthOutput) { n := int32(2); o.Probe.AudioChannels = &n },
		func(o *toolapp.DepthOutput) { c := "vp9"; o.Probe.Codec = &c },
	} {
		o := depthJobOutput(f)
		mutate(o)
		if !errors.Is(toolapp.ValidateDepthReceipt(f, o), toolapp.ErrDepthOutputInvalid) {
			t.Fatal("unverified native contract accepted", o.Receipt)
		}
	}
	oversized := f
	oversized.Input.ByteSize = (500 << 20) + 1
	if oversized.Validate() == nil {
		t.Fatal("oversized frozen source accepted")
	}
	long := int32(15101)
	f.Input.DurationMS = &long
	if f.Validate() == nil {
		t.Fatal("source duration truncated instead of rejected")
	}
}
func TestDepthJobExecutionUnknownCannotBeRetriedOrCancelledEarly(t *testing.T) {
	s := tooldomain.DepthExecution{Job: tooldomain.DepthJob{ID: uuid.New(), ProjectID: uuid.New(), ProfileID: tooldomain.DepthProfileID, Status: tooldomain.DepthQueued, Stage: "queued", Attempt: 1, Revision: 1}}
	w := uuid.New()
	if err := s.Claim(w, false); err != nil {
		t.Fatal(err)
	}
	if err := s.StartProcess(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Interrupt(w); err != nil {
		t.Fatal(err)
	}
	if !s.Job.ExecutionUnconfirmed || s.WorkerID != w || s.Retry() == nil {
		t.Fatal("timeout invented process cessation")
	}
	if err := s.RequestCancel(); err != nil {
		t.Fatal(err)
	}
	if s.FinishCancelled(w) == nil || s.Claim(uuid.New(), true) == nil {
		t.Fatal("unknown worker released by control")
	}
	if err := s.EndProcess(w); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCancelled(w); err != nil {
		t.Fatal(err)
	}
	if s.Job.Status != tooldomain.DepthCancelled || s.WorkerID != uuid.Nil || s.Job.ExecutionUnconfirmed {
		t.Fatal("actual cessation not preserved")
	}
	if err := s.Retry(); err != nil {
		t.Fatal(err)
	}
	if s.Job.Attempt != 2 || s.Job.Status != tooldomain.DepthQueued {
		t.Fatal("retry did not fence old attempt")
	}
	if err := s.EndProcess(w); err == nil {
		t.Fatal("old worker accepted")
	}
	raw, err := json.Marshal(s.Job)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"worker", "object_key", "receipt", "native"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private execution exposed", private)
		}
	}
}
