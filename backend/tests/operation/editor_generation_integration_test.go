package operation_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type editorWireTransport func(*http.Request) (*http.Response, error)

func (f editorWireTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type editorMediaSigner struct{}

func (editorMediaSigner) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://editor-fixture.invalid/" + key, nil
}

// This uses real stores, compiler, dispatch guard and HTTP serialization, with
// an in-memory transport. It is not evidence of a live supplier or image quality.
func TestEditorSavedReferencesReachManagedWireOnce(t *testing.T) {
	db := operationStoreDB(t)
	f := seedManagedFixture(t, db)
	actor, project := operationStoreProject(t, db)
	version, mode := f.identity.ModelProfileVersionID.String(), "image2image"
	require.NoError(t, db.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, project).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.capability SET modes=array_append(modes,'image2image'),input_roles=ARRAY['prompt','reference_image'] WHERE key='image.generate' AND NOT ('image2image'=ANY(modes))`).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['image2image'],queue='provider',limits='{"max_outputs":1,"roles":{"reference_image":{"types":["image"],"max_count":4,"max_bytes":1000}}}' WHERE id=?`, f.identity.ModelProfileVersionID).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET rule='{"base_micros":1}' WHERE id=?`, f.identity.PriceRuleVersionID).Error)
	// The active release is authoritative even if current_version_id points to a
	// newer draft or is unset. The project model list already follows the release.
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET current_version_id=NULL WHERE active_release_id=?`, f.release).Error)
	draft := promptdomain.Draft{
		SchemaVersion: 1, ModelVersion: &version, Mode: &mode, Params: map[string]any{},
		LabelState: promptdomain.LabelState{
			HighWater: map[string]int64{"image": 2, "video": 0, "audio": 0, "subject": 0},
			Allocations: map[string]promptdomain.LabelAllocation{
				"a": {LabelKind: "image", Ordinal: 1},
				"b": {LabelKind: "image", Ordinal: 2},
			},
		},
		Document: promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{
			Type: "paragraph", Content: []promptdomain.Inline{
				{Type: "text", Text: "使用"},
				{Type: "reference", Attrs: &promptdomain.ReferenceAttrs{ReferenceID: "b"}},
				{Type: "reference", Attrs: &promptdomain.ReferenceAttrs{ReferenceID: "a"}},
				{Type: "reference", Attrs: &promptdomain.ReferenceAttrs{ReferenceID: "b"}},
			},
		}}},
	}
	keys := []string{}
	for index, ref := range []string{"b", "a"} {
		asset := uuid.New()
		key := "projects/" + project.String() + "/" + asset.String() + ".png"
		keys = append(keys, key)
		require.NoError(t, db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,'reference.png','image/png',50,repeat('a',64),'passed')`, asset, project, key).Error)
		draft.References = append(draft.References, promptdomain.Reference{ID: ref, BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: index, Source: promptdomain.ReferenceSource{Kind: "media", SourceID: asset.String(), SourceVersion: "1"}})
	}
	saved, err := editorDraftStore(db).SaveDraft(t.Context(), actor, promptapp.DraftWrite{ProjectID: project, Key: uuid.New(), Draft: draft})
	require.NoError(t, err)
	store := editorOperationStore(db)
	epoch := int64(0)
	quote, err := store.CreateFreeQuote(t.Context(), actor, operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{InputSource: &promptdomain.InputSource{Kind: "prompt_draft", DraftID: saved.DraftID, Revision: saved.Revision}, ExpectedSubmissionEpoch: &epoch}})
	require.NoError(t, err)
	require.Equal(t, "使用[图片2][图片1][图片2]", quote.FinalPrompt)
	require.Equal(t, []promptdomain.ReferenceMapping{
		{ReferenceID: "b", Label: "图片2", Role: "reference_image", MediaSeqNos: []int{1}},
		{ReferenceID: "a", Label: "图片1", Role: "reference_image", MediaSeqNos: []int{2}},
	}, quote.ReferenceMapping)
	_, err = store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: quote.ConfirmKey.String()})
	require.NoError(t, err)
	// Later edits and mode changes must not alter the accepted operation input.
	draft.Document.Content[0].Content[0].Text = "确认后的新意图"
	nextMode := "text2image"
	draft.Mode = &nextMode
	draft.References[0], draft.References[1] = draft.References[1], draft.References[0]
	draft.References[0].Order, draft.References[1].Order = 0, 1
	_, err = editorDraftStore(db).SaveDraft(t.Context(), actor, promptapp.DraftWrite{ProjectID: project, DraftID: saved.DraftID, ExpectedRevision: saved.Revision, Key: uuid.New(), Draft: draft})
	require.NoError(t, err)
	requestKey := "lv-" + quote.OperationID.String()
	_, err = store.TransitionWorkflowOperation(t.Context(), operationapp.TransitionInput{OperationID: quote.OperationID, From: []operationdomain.Status{operationdomain.StatusConfirmed}, To: operationdomain.StatusSubmitting, ProviderRequestKey: &requestKey})
	require.NoError(t, err)
	require.NoError(t, store.BeginProviderCall(t.Context(), operationapp.BeginProviderCallInput{OperationID: quote.OperationID, Action: "submit", Attempt: 1, DispatchRequired: true}))
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 1, 1))))
	response, err := json.Marshal(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(encoded.Bytes()), "media_type": "image/png"}}})
	require.NoError(t, err)
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: editorWireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			Prompt     string `json:"prompt"`
			References []struct {
				Type  string            `json:"type"`
				Image map[string]string `json:"image_url"`
			} `json:"input_references"`
		}
		require.Equal(t, "/api/v1/images", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, quote.FinalPrompt, payload.Prompt)
		require.Len(t, payload.References, 2)
		for index, ref := range payload.References {
			require.Equal(t, "image_url", ref.Type)
			require.Equal(t, "https://editor-fixture.invalid/"+keys[index], ref.Image["url"])
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(response)))}, nil
	})})
	results := operationapp.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	worker := managed.NewActivities(store, fixtureUnsealer{}, client, results, editorMediaSigner{}, nil)
	progress, err := worker.Step(t.Context(), operationapp.ManagedStep{OperationID: quote.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, "succeeded", progress.State)
	replay, err := worker.Step(t.Context(), operationapp.ManagedStep{OperationID: quote.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, progress, replay)
	require.Equal(t, 1, calls)
}
