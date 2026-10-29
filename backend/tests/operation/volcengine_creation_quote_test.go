package operation_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/google/uuid"
)

func TestVisualQuoteUsesFrozenDrivingDurationWithoutPrompt(t *testing.T) {
	for _, tc := range []struct {
		adapter, mode, role, kind string
		duration                  int64
	}{
		{"volcengine_avatar", "avatar", "driving_audio", "audio", 15500},
		{"volcengine_motion", "motion", "driving_video", "video", 29500},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			candidate := catalog.BuiltinModelCatalog(tc.adapter).Models[0]
			version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			contract := operation.FreeQuoteCatalog{ModelVersionID: version.ID, ProviderModelID: version.ProviderModelID, Region: "domestic", Modes: version.Modes, Limits: version.Limits, ParamSchema: version.ParamSchema, InputRoles: []string{"prompt", "character_image", tc.role}, Price: catalog.PriceRuleVersion{ID: uuid.New(), ModelID: version.ModelID, VersionNo: 1, Unit: catalog.PricePerSecond, Rule: json.RawMessage(`{"base_micros":1000}`), Currency: "CNY", EffectiveFrom: now.Add(-time.Minute)}}
			image, drive := uuid.New(), uuid.New()
			input := operation.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: operation.FreeQuoteItemInput{ModelKey: candidate.Key, Capability: "video.generate", Mode: tc.mode, Params: json.RawMessage(`{}`), OutputCount: 1, MediaInputs: []operation.FreeQuoteMediaInput{{Role: "character_image", MediaAssetID: image}, {Role: tc.role, MediaAssetID: drive}}}}
			size := int32(720)
			facts := []operation.FreeQuoteMediaFact{{ID: image, Kind: "image", MIMEType: "image/png", Width: &size, Height: &size, SHA256: strings.Repeat("a", 64), ByteSize: 1000}, {ID: drive, Kind: tc.kind, MIMEType: "video/mp4", Width: &size, Height: &size, SHA256: strings.Repeat("b", 64), ByteSize: 1000, DurationMS: &tc.duration}}
			got, err := operation.PrepareFreeQuote(input, contract, facts, now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Inputs) != 2 || got.Inputs[0].TextValue != nil || *got.Operation.QuoteMicros != ((tc.duration+999)/1000)*1000 {
				t.Fatalf("wrong frozen inputs/quote: %+v", got)
			}
			input.Params = json.RawMessage(`{"duration_ms":1000}`)
			if _, err := operation.PrepareFreeQuote(input, contract, facts, now, nil); err == nil {
				t.Fatal("client-controlled duration admitted")
			}
			input.Params = json.RawMessage(`{}`)
			tc.duration = 60000
			if _, err := operation.PrepareFreeQuote(input, contract, facts, now, nil); err == nil {
				t.Fatal("out-of-range driving media admitted")
			}
		})
	}
}
