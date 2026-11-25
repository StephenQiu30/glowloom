package media_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestReferenceVoiceValidation(t *testing.T) {
	duration := int32(30000)
	asset := application.AssetSummary{ID: uuid.New(), Kind: "audio", MIMEType: "audio/wav", ByteSize: 10 * 1024 * 1024, DurationMS: &duration, Revision: 1}
	if err := application.ValidateReferenceVoice("旁白", true, asset); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*application.AssetSummary){func(a *application.AssetSummary) { a.Kind = "video" }, func(a *application.AssetSummary) { a.DurationMS = nil }, func(a *application.AssetSummary) { d := int32(30001); a.DurationMS = &d }, func(a *application.AssetSummary) { a.ByteSize++ }} {
		invalid := asset
		change(&invalid)
		if application.ValidateReferenceVoice("旁白", true, invalid) == nil {
			t.Fatal("invalid sample accepted")
		}
	}
	if application.ValidateReferenceVoice("", true, asset) == nil || application.ValidateReferenceVoice("旁白", false, asset) == nil {
		t.Fatal("name/consent missing")
	}
}
