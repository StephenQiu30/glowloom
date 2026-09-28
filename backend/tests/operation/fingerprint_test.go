package operation_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func fingerprintFixture() application.FingerprintInput {
	assetID := uuid.MustParse("70e85a02-277f-4b69-bef2-8860b4df0bc5")
	return application.FingerprintInput{
		Capability: "image.generate", Mode: "image2image",
		ModelProfileVersionID: uuid.MustParse("63784d2c-026e-4288-9efa-3157cf0a335e"),
		ProviderModelID:       "provider-image-v1", OutputCount: 2,
		Params: json.RawMessage(`{"seed":42,"guidance":1.0,"resolution":"1080p"}`),
		ParamSchema: json.RawMessage(`[
			{"field":"seed","type":"integer"},
			{"field":"guidance","type":"number"},
			{"field":"resolution","type":"string","default":"1080p"}
		]`),
		Inputs: []application.FingerprintPart{
			{SeqNo: 0, Role: "subject", RefType: "media_asset", RefID: &assetID,
				RefVersion: "v2", MediaSHA256: strings.Repeat("a", 64)},
			{SeqNo: 1, Role: "prompt", RefType: "text", TextValue: "一只红色的猫"},
		},
		FinalPrompt: "一只红色的猫，电影感",
	}
}

func TestInputHashNormalizesDefaultsNumbersAndInputOrder(t *testing.T) {
	base := fingerprintFixture()
	want, reusable, err := application.InputHash(base)
	if err != nil || !reusable || len(want) != 64 {
		t.Fatalf("base input hash = %q, reusable %t, error %v", want, reusable, err)
	}
	equivalent := base
	equivalent.Params = json.RawMessage(`{"guidance":1e0,"seed":42}`)
	equivalent.Inputs = []application.FingerprintPart{base.Inputs[1], base.Inputs[0]}
	equivalent.Inputs[0].SeqNo = 11
	equivalent.Inputs[1].SeqNo = 10
	got, reusable, err := application.InputHash(equivalent)
	if err != nil || !reusable || got != want {
		t.Fatalf("equivalent input hash = %q, reusable %t, error %v; want %q", got, reusable, err, want)
	}
}

func TestInputHashChangesWhenSubmittedOutputChanges(t *testing.T) {
	base := fingerprintFixture()
	want, _, err := application.InputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*application.FingerprintInput)
	}{
		{"capability", func(v *application.FingerprintInput) { v.Capability = "video.generate" }},
		{"mode", func(v *application.FingerprintInput) { v.Mode = "text2image" }},
		{"model version", func(v *application.FingerprintInput) { v.ModelProfileVersionID = uuid.New() }},
		{"provider model", func(v *application.FingerprintInput) { v.ProviderModelID = "provider-image-v2" }},
		{"output count", func(v *application.FingerprintInput) { v.OutputCount = 1 }},
		{"parameter", func(v *application.FingerprintInput) { v.Params = json.RawMessage(`{"seed":42,"guidance":1.5}`) }},
		{"input role", func(v *application.FingerprintInput) { v.Inputs[0].Role = "style" }},
		{"input reference", func(v *application.FingerprintInput) { id := uuid.New(); v.Inputs[0].RefID = &id }},
		{"input version", func(v *application.FingerprintInput) { v.Inputs[0].RefVersion = "v3" }},
		{"media contents", func(v *application.FingerprintInput) { v.Inputs[0].MediaSHA256 = strings.Repeat("b", 64) }},
		{"mask contents", func(v *application.FingerprintInput) { v.Inputs[0].MaskSHA256 = strings.Repeat("c", 64) }},
		{"frozen text", func(v *application.FingerprintInput) { v.Inputs[1].TextValue = "一只蓝色的猫" }},
		{"final prompt", func(v *application.FingerprintInput) { v.FinalPrompt = "用户改写的提示词" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			changed.Inputs = append([]application.FingerprintPart(nil), base.Inputs...)
			tc.change(&changed)
			got, reusable, err := application.InputHash(changed)
			if err != nil || !reusable || got == want {
				t.Fatalf("changed input hash = %q, reusable %t, error %v; base %q", got, reusable, err, want)
			}
		})
	}
}

func TestInputHashDistinguishesMediaAndMaskAssetIdentity(t *testing.T) {
	base := fingerprintFixture()
	mediaID, maskID := uuid.New(), uuid.New()
	base.Inputs[0].MediaAssetID = &mediaID
	base.Inputs[0].MaskAssetID = &maskID
	base.Inputs[0].MaskSHA256 = strings.Repeat("c", 64)
	want, reusable, err := application.InputHash(base)
	if err != nil || !reusable {
		t.Fatalf("base media input hash = %q, reusable %t, error %v", want, reusable, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*application.FingerprintPart)
	}{
		{"media asset", func(part *application.FingerprintPart) { id := uuid.New(); part.MediaAssetID = &id }},
		{"mask asset", func(part *application.FingerprintPart) { id := uuid.New(); part.MaskAssetID = &id }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			changed.Inputs = append([]application.FingerprintPart(nil), base.Inputs...)
			tc.change(&changed.Inputs[0])
			got, reusable, err := application.InputHash(changed)
			if err != nil || !reusable || got == want {
				t.Fatalf("changed media input hash = %q, reusable %t, error %v; base %q", got, reusable, err, want)
			}
		})
	}
}

func TestInputHashMarksUnspecifiedSeedAsNonReusable(t *testing.T) {
	input := fingerprintFixture()
	input.Params = json.RawMessage(`{"guidance":1}`)
	_, reusable, err := application.InputHash(input)
	if err != nil || reusable {
		t.Fatalf("missing seed reusable = %t, error %v", reusable, err)
	}
	input.ParamSchema = json.RawMessage(`[
		{"field":"seed","type":"integer","default":42},
		{"field":"guidance","type":"number"}
	]`)
	randomHash, reusable, err := application.InputHash(input)
	if err != nil || reusable {
		t.Fatalf("unspecified seed with schema default reusable = %t, error %v", reusable, err)
	}
	input.Params = json.RawMessage(`{"seed":42,"guidance":1}`)
	fixedHash, reusable, err := application.InputHash(input)
	if err != nil || !reusable || fixedHash == randomHash {
		t.Fatalf("explicit default seed hash = %q, reusable %t, error %v; random %q", fixedHash, reusable, err, randomHash)
	}
	input.Params = json.RawMessage(`{"guidance":1}`)
	input.ParamSchema = json.RawMessage(`[{"field":"guidance","type":"number"}]`)
	_, reusable, err = application.InputHash(input)
	if err != nil || !reusable {
		t.Fatalf("model without seed reusable = %t, error %v", reusable, err)
	}
	input.ParamSchema = json.RawMessage(` [ ] `)
	input.Params = json.RawMessage(`{}`)
	_, reusable, err = application.InputHash(input)
	if err != nil || !reusable {
		t.Fatalf("empty schema reusable = %t, error %v", reusable, err)
	}
	input.Params = json.RawMessage(`{"guidance":1}`)
	input.ParamSchema = json.RawMessage(`[
		{"field":"guidance","type":"number"},
		{"field":"seed","type":"integer","for_modes":["video.generate"]}
	]`)
	_, reusable, err = application.InputHash(input)
	if err != nil || !reusable {
		t.Fatalf("seed outside active mode reusable = %t, error %v", reusable, err)
	}
}

func TestInputHashRejectsAmbiguousOrInvalidFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*application.FingerprintInput)
	}{
		{"duplicate parameter", func(v *application.FingerprintInput) { v.Params = json.RawMessage(`{"seed":1,"seed":2}`) }},
		{"unknown parameter", func(v *application.FingerprintInput) { v.Params = json.RawMessage(`{"unknown":1}`) }},
		{"null seed", func(v *application.FingerprintInput) { v.Params = json.RawMessage(`{"seed":null}`) }},
		{"duplicate sequence", func(v *application.FingerprintInput) { v.Inputs[1].SeqNo = 0 }},
		{"invalid media hash", func(v *application.FingerprintInput) { v.Inputs[0].MediaSHA256 = "not-a-sha256" }},
		{"missing media digest", func(v *application.FingerprintInput) {
			id := uuid.New()
			v.Inputs[0].MediaAssetID = &id
			v.Inputs[0].MediaSHA256 = ""
		}},
		{"missing mask digest", func(v *application.FingerprintInput) { id := uuid.New(); v.Inputs[0].MaskAssetID = &id }},
		{"zero output count", func(v *application.FingerprintInput) { v.OutputCount = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := fingerprintFixture()
			input.Inputs = append([]application.FingerprintPart(nil), input.Inputs...)
			tc.change(&input)
			if _, _, err := application.InputHash(input); !errors.Is(err, application.ErrInvalidFingerprint) {
				t.Fatalf("invalid input error = %v; want ErrInvalidFingerprint", err)
			}
		})
	}
}
