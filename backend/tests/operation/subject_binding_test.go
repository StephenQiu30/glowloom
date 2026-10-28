package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type subjectNoDispatchTransport struct{ t *testing.T }

func (s subjectNoDispatchTransport) RoundTrip(*http.Request) (*http.Response, error) {
	s.t.Fatal("unsupported subject media was dispatched as an image")
	return nil, nil
}

func TestProviderRejectsUnsupportedSubjectMediaBeforeDispatch(t *testing.T) {
	client := providerhttp.NewClient(&http.Client{Transport: subjectNoDispatchTransport{t}})
	for _, kind := range []string{"audio", "video"} {
		result, err := client.Submit(context.Background(), operationapp.GenerationRequest{Config: catalogdomain.BuiltinPresets()[1].Config, ModelID: "fixture/model", Capability: "image.generate", Mode: "image2image", Prompt: "主体", OutputCount: 1, References: []operationapp.ProviderReference{{Role: "subject", URL: "https://assets.example/media", MediaKind: kind}}}, "fixture-key")
		if err != nil || result.Outcome != "not_submitted" {
			t.Fatalf("unsupported %s: %+v, %v", kind, result, err)
		}
	}
}

func TestQuoteFreezesSubjectBindingProofAndRejectsUnprovedBinding(t *testing.T) {
	now := time.Now().UTC()
	asset, binding := uuid.New(), uuid.New()
	input := operationapp.CreateFreeQuoteInput{ProjectID: uuid.New(), RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{
		ModelKey: "image.mock", Capability: "image.generate", Mode: "text_to_image", Prompt: "主体", Params: json.RawMessage(`{}`), OutputCount: 1,
		MediaInputs: []operationapp.FreeQuoteMediaInput{{Role: "subject", MediaAssetID: asset, SubjectBindingID: &binding}},
	}}
	catalog := operationapp.FreeQuoteCatalog{ModelVersionID: uuid.New(), ProviderModelID: "mock", Region: "domestic", Modes: []string{"text_to_image"}, InputRoles: []string{"subject"},
		Limits: json.RawMessage(`{"max_outputs":1,"roles":{"subject":{"max_count":8,"types":["image"]}}}`), ParamSchema: json.RawMessage(`[]`),
		Price: catalogdomain.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: catalogdomain.PricePerImage, Rule: json.RawMessage(`{"base_micros":1}`), Currency: "CNY", EffectiveFrom: now}}
	facts := []operationapp.FreeQuoteMediaFact{{ID: asset, Kind: "image", SHA256: strings.Repeat("a", 64), ByteSize: 20, SubjectBindingID: &binding, SubjectBindingSHA256: strings.Repeat("b", 64)}}
	prepared, err := operationapp.PrepareFreeQuote(input, catalog, facts, now, nil)
	if err != nil || len(prepared.Inputs) != 2 || prepared.Inputs[1].RefType != "subject_media" || prepared.Inputs[1].RefID == nil || *prepared.Inputs[1].RefID != binding || prepared.Inputs[1].RefVersion == nil || *prepared.Inputs[1].RefVersion != strings.Repeat("b", 64) {
		t.Fatalf("subject proof not frozen: %+v, %v", prepared, err)
	}
	facts[0].SubjectBindingID = nil
	if _, err := operationapp.PrepareFreeQuote(input, catalog, facts, now, nil); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("unproved subject binding accepted: %v", err)
	}
}

func TestLegacyMediaRequestHashEncodingRemainsUnchanged(t *testing.T) {
	asset := uuid.MustParse("27b104d8-c010-4e6e-ae19-904775541011")
	raw, err := json.Marshal(operationapp.FreeQuoteMediaInput{Role: "subject", MediaAssetID: asset})
	if err != nil || string(raw) != `{"Role":"subject","MediaAssetID":"27b104d8-c010-4e6e-ae19-904775541011"}` {
		t.Fatalf("legacy permanent request encoding changed: %s, %v", raw, err)
	}
}
