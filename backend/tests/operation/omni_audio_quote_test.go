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

func TestOmniAudioQuoteReservesMaximumAndValidatesFrozenMedia(t *testing.T) {
	candidate := catalog.BuiltinModelCatalog("volcengine_speech").Models[1]
	v, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contract := operation.FreeQuoteCatalog{ModelVersionID: v.ID, ProviderModelID: v.ProviderModelID, Region: "domestic", Modes: v.Modes, Limits: v.Limits, ParamSchema: v.ParamSchema, InputRoles: []string{"prompt", "reference_audio", "reference_image"}, Price: catalog.PriceRuleVersion{ID: uuid.New(), ModelID: v.ModelID, VersionNo: 1, Unit: catalog.PricePerSecond, Rule: json.RawMessage(`{"base_micros":1000}`), Currency: "CNY", EffectiveFrom: now.Add(-time.Minute)}}
	in := operation.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: operation.FreeQuoteItemInput{ModelKey: candidate.Key, Capability: "audio.generate", Mode: "omni_audio", Prompt: "生成10秒海浪声", Params: json.RawMessage(`{}`), OutputCount: 1}}
	got, err := operation.PrepareFreeQuote(in, contract, nil, now, nil)
	if err != nil || *got.Operation.QuoteMicros != 120000 {
		t.Fatalf("quote=%+v err=%v", got, err)
	}
	id := uuid.New()
	duration := int64(30001)
	in.MediaInputs = []operation.FreeQuoteMediaInput{{Role: "reference_audio", MediaAssetID: id}}
	facts := []operation.FreeQuoteMediaFact{{ID: id, Kind: "audio", MIMEType: "audio/mpeg", SHA256: strings.Repeat("a", 64), ByteSize: 1000, DurationMS: &duration}}
	if _, err := operation.PrepareFreeQuote(in, contract, facts, now, nil); err == nil {
		t.Fatal("accepted overlong frozen reference")
	}
	duration = 30000
	if _, err := operation.PrepareFreeQuote(in, contract, facts, now, nil); err != nil {
		t.Fatal(err)
	}
	in.Params = json.RawMessage(`{"duration_ms":1000}`)
	if _, err := operation.PrepareFreeQuote(in, contract, facts, now, nil); err == nil {
		t.Fatal("allowed client to reduce reservation with unsupported parameter")
	}
}
