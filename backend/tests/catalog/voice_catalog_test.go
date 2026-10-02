package catalog_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const voiceCatalogSchema = `[{"field":"voice_id","label":"音色","type":"string","component":"voice","enum":["z_voice","a_voice"]},{"field":"speed","label":"语速","type":"number","component":"slider","min":0.5,"max":2,"step":0.1,"default":1},{"field":"emotion","label":"情绪","type":"string","component":"select","enum":["neutral","happy"]}]`

type voiceModelsStub struct {
	models []catalogapp.ConfiguredVoiceModel
	err    error
	calls  int
}

func (s *voiceModelsStub) ReadConfiguredVoiceModels(_ context.Context, _ identityapp.Principal, _ uuid.UUID, input catalogapp.VoiceModelInput) ([]catalogapp.ConfiguredVoiceModel, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	var rows []catalogapp.ConfiguredVoiceModel
	for _, row := range s.models {
		if input.ModelKey != "" && row.Key != input.ModelKey || input.After != nil && (row.Key < input.After.Key || row.Key == input.After.Key && !input.After.Inclusive) {
			continue
		}
		rows = append(rows, row)
		if len(rows) == input.Limit {
			break
		}
	}
	return rows, nil
}

func voiceCatalogFixture(t *testing.T) (*catalogapp.VoiceCatalog, *voiceModelsStub, identityapp.Principal, uuid.UUID) {
	t.Helper()
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	store := &voiceModelsStub{models: []catalogapp.ConfiguredVoiceModel{{Key: "configured-tts", DisplayName: "已配置音色", Version: domain.ModelVersion{
		ID: uuid.New(), ModelID: uuid.New(), VersionNo: 2, ProviderModelID: "configured-v2", Modes: []string{"tts"},
		Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(voiceCatalogSchema),
		ExpectedMaxMS: 1000, Queue: "media", Moderation: domain.ModerationPlatform,
	}}}}
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	return catalogapp.NewVoiceCatalog(store, validator), store, actor, uuid.New()
}

func TestVoiceCatalogFreezesActualVersionAndOnlyProvidedParams(t *testing.T) {
	query, store, actor, project := voiceCatalogFixture(t)
	fact, err := query.Reference(t.Context(), actor, project, catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 2, VoiceKey: "a_voice", Params: json.RawMessage(`{"emotion":"happy","speed":1.2}`)})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(store.models[0].Version.ParamSchema)
	if fact.ModelVersionID != store.models[0].Version.ID || fact.ModelVersion != 2 || fact.VoiceKey != "a_voice" || fact.ParamSchemaSHA256 != hex.EncodeToString(sum[:]) || string(fact.Params) != `{"emotion":"happy","speed":1.2}` {
		t.Fatalf("wrong frozen fact: %+v", fact)
	}
	empty, err := query.Reference(t.Context(), actor, project, catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 2, VoiceKey: "z_voice", Params: json.RawMessage(`{}`)})
	if err != nil || string(empty.Params) != `{}` {
		t.Fatalf("defaults were materialized or empty selection refused: %+v %v", empty, err)
	}
}

func TestVoiceCatalogRejectsStaleUnknownMalformedAndInvalidParams(t *testing.T) {
	for name, params := range map[string]string{
		"undeclared": `{"pitch":1}`, "unknown enum": `{"emotion":"sad"}`, "below minimum": `{"speed":0.4}`,
		"above maximum": `{"speed":2.1}`, "off step": `{"speed":1.25}`, "string number": `{"speed":"1.2"}`,
		"duplicate field": `{"speed":1,"speed":1.2}`, "trailing data": `{} {}`, "non object": `[]`, "null": `null`,
		"zero large exponent": `{"speed":0e1000000}`, "tiny large exponent": `{"speed":1e-1000000}`,
	} {
		t.Run(name, func(t *testing.T) {
			q, _, actor, project := voiceCatalogFixture(t)
			_, err := q.Reference(t.Context(), actor, project, catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 2, VoiceKey: "a_voice", Params: json.RawMessage(params)})
			if !errors.Is(err, catalogapp.ErrInvalidVoiceSelection) {
				t.Fatalf("invalid parameters accepted: %v", err)
			}
		})
	}
	q, store, actor, project := voiceCatalogFixture(t)
	_, err := q.Reference(t.Context(), actor, project, catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 1, VoiceKey: "a_voice", Params: json.RawMessage(`{}`)})
	if !errors.Is(err, catalogapp.ErrVoiceVersionConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	_, err = q.Reference(t.Context(), actor, project, catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 2, VoiceKey: "unknown", Params: json.RawMessage(`{}`)})
	if !errors.Is(err, catalogapp.ErrInvalidVoiceSelection) {
		t.Fatalf("unknown voice accepted: %v", err)
	}
	store.models[0].Version.ParamSchema = json.RawMessage(`[{"field":"voice_id","label":"音色","type":"string","component":"voice","enum":["a"]},{"field":"voice_id","label":"重复","type":"string","component":"voice","enum":["b"]}]`)
	_, err = q.List(t.Context(), actor, project, 10, nil)
	if !errors.Is(err, catalogapp.ErrVoiceCatalogUnavailable) {
		t.Fatalf("corrupt published schema became usable/empty: %v", err)
	}
}

func TestVoiceCatalogFiltersAfterCompleteModelPagesAndNeverTruncatesEnums(t *testing.T) {
	q, store, actor, project := voiceCatalogFixture(t)
	base := store.models[0]
	store.models = nil
	for i := range 51 {
		model := base
		model.Key = fmt.Sprintf("tts-%03d", i)
		if i < 50 {
			model.Version.ParamSchema = json.RawMessage(`[]`)
		}
		store.models = append(store.models, model)
	}
	page, err := q.List(t.Context(), actor, project, 2, nil)
	if err != nil || len(page.Voices) != 2 || page.Next != nil || store.calls != 2 || page.Voices[0].ModelKey != "tts-050" || page.Voices[0].VoiceKey != "a_voice" || page.Voices[1].VoiceKey != "z_voice" {
		t.Fatalf("filtered model page became a false empty/truncated voice list: %+v calls=%d %v", page, store.calls, err)
	}
}

func TestVoiceCatalogRequiresOneCompatiblePublishedModeAndExplicitRequiredParam(t *testing.T) {
	q, store, actor, project := voiceCatalogFixture(t)
	store.models[0].Version.Modes = []string{"tts", "preview"}
	store.models[0].Version.ParamSchema = json.RawMessage(`[{"field":"voice_id","label":"音色","type":"string","component":"voice","enum":["a_voice"],"for_modes":["tts"]},{"field":"speed","label":"语速","type":"number","component":"slider","min":0.5,"max":2,"step":0.1,"required":true,"for_modes":["preview"]}]`)
	input := catalogapp.VoiceSelection{ModelKey: "configured-tts", ExpectedVersion: 2, VoiceKey: "a_voice", Params: json.RawMessage(`{"speed":1.2}`)}
	if _, err := q.Reference(t.Context(), actor, project, input); !errors.Is(err, catalogapp.ErrInvalidVoiceSelection) {
		t.Fatalf("voice/params from incompatible modes bound together: %v", err)
	}
	store.models[0].Version.ParamSchema = json.RawMessage(`[{"field":"voice_id","label":"音色","type":"string","component":"voice","enum":["a_voice"]},{"field":"speed","label":"语速","type":"number","component":"slider","min":0.5,"max":2,"step":0.1,"required":true}]`)
	input.Params = json.RawMessage(`{}`)
	if _, err := q.Reference(t.Context(), actor, project, input); !errors.Is(err, catalogapp.ErrInvalidVoiceSelection) {
		t.Fatalf("required voice parameter guessed instead of supplied: %v", err)
	}
	input.Params = json.RawMessage(`{"speed":1.2}`)
	if _, err := q.Reference(t.Context(), actor, project, input); err != nil {
		t.Fatal("complete explicit parameter refused", err)
	}
}

func TestVoiceCatalogPagesSortedRealChoicesAndPreservesOwnerErrors(t *testing.T) {
	q, store, actor, project := voiceCatalogFixture(t)
	page, err := q.List(t.Context(), actor, project, 1, nil)
	if err != nil || len(page.Voices) != 1 || page.Voices[0].VoiceKey != "a_voice" || page.Next == nil {
		t.Fatalf("wrong first page: %+v %v", page, err)
	}
	page, err = q.List(t.Context(), actor, project, 1, page.Next)
	if err != nil || len(page.Voices) != 1 || page.Voices[0].VoiceKey != "z_voice" || page.Next != nil {
		t.Fatalf("wrong second page: %+v %v", page, err)
	}
	store.err = identityapp.ErrForbidden
	_, err = q.List(t.Context(), actor, project, 10, nil)
	if !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("lost owning authorization error: %v", err)
	}
	store.err = nil
	actor.MustChangePassword = true
	before := store.calls
	_, err = q.List(t.Context(), actor, project, 10, nil)
	if !errors.Is(err, identityapp.ErrForbidden) || store.calls != before {
		t.Fatalf("invalid actor reached owner: %v", err)
	}
}
