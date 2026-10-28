package subject_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type actualSubjectGeneration struct {
	runtime, owner  *gorm.DB
	uploader, actor identity.Principal
	project         uuid.UUID
	storage         *subjectObjectServer
	key             string
	original        []byte
	subject         subject.Detail
	quote           operation.CreateFreeQuoteResult
	input           operationdomain.OperationInput
	store           *pgoperation.Store
}

func actualSubjectDraftStore(db *gorm.DB) *pgprompt.DraftStore {
	return pgprompt.NewDraftStore(db, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources)
}

func actualSubjectOperationStore(db *gorm.DB, objects subjectwire.Objects) *pgoperation.Store {
	return pgoperation.NewScopedStoreWithEditor(db, func(tx *gorm.DB) operation.QuotePromptCompiler {
		return prompt.NewCompiler(operationwire.ProvidePromptPreferenceStore(tx))
	}, func(tx *gorm.DB) operation.EditorSnapshots {
		return actualSubjectDraftStore(tx)
	}, operationwire.ProvideOperationProjectAccess, operationwire.ProvideOperationWorkerAccess, operationwire.ProvideOperationPlatformAccess).WithSubjectMedia(func(tx *gorm.DB) operation.SubjectMediaAccess {
		return subjectwire.NewGenerationAccess(tx, objects)
	})
}

// The release and price are real SQL contracts, with a synthetic unopened
// credential. No supplier is called, and no credential decryption is claimed.
func seedActualSubjectCatalog(t *testing.T, owner *gorm.DB, project uuid.UUID) (versionID, releaseID uuid.UUID) {
	t.Helper()
	provider, model, price, configID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	versionID, releaseID = uuid.New(), uuid.New()
	capability := "image.generate.subject-fixture-" + uuid.NewString()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status) VALUES(?,?,'Subject actual fixture','volcengine_ark','domestic','active')`, []any{provider, "subject-fixture-" + provider.String()}},
		{`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4,status) VALUES(?,?,'synthetic fixture',?,'synthetic-never-decrypted','test','active')`, []any{uuid.New(), provider, []byte("synthetic-not-a-real-secret")}},
		{`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,?,'image',ARRAY['image2image'],ARRAY['prompt','reference_image'])`, []any{uuid.New(), capability}},
		{`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,?,'Subject fixture model','active')`, []any{model, "subject-model-" + model.String(), provider, capability}},
		{`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms) VALUES(?,?,1,'subject-fixture-model',ARRAY['image2image'],'{"max_outputs":1,"roles":{"reference_image":{"min_count":1,"max_count":2,"types":["image"],"max_bytes":1048576}}}'::jsonb,'[]'::jsonb,true,false,false,30000)`, []any{versionID, model}},
		{`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,effective_from) VALUES(?,?,1,'per_image','{"base_micros":1}'::jsonb,?)`, []any{price, model, time.Now().UTC().Add(-time.Hour)}},
		{`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) SELECT ?,?,id,1,payload->'config',content_hash FROM catalog.provider_preset_version WHERE id='6da07a12-0000-4000-8000-000000000001'`, []any{configID, provider}},
		{`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost,status) VALUES(?,?,?,?,?,'{}'::jsonb,'active')`, []any{releaseID, model, versionID, price, configID}},
		{`UPDATE catalog.model_profile SET active_release_id=?,current_version_id=? WHERE id=?`, []any{releaseID, versionID, model}},
		{`INSERT INTO billing.budget(id,project_id,limit_micros) VALUES(?,?,1000000)`, []any{uuid.New(), project}},
	}
	for _, statement := range statements {
		result := owner.Exec(statement.query, statement.args...)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("seed own actual subject catalog: rows=%d err=%v", result.RowsAffected, result.Error)
		}
	}
	return versionID, releaseID
}

func newActualSubjectGeneration(t *testing.T) actualSubjectGeneration {
	t.Helper()
	runtime, owner := subjectPostgres(t)
	var mutableInputs bool
	if err := runtime.Raw(`SELECT has_table_privilege(current_user,'operation.operation_input','UPDATE,DELETE')`).Scan(&mutableInputs).Error; err != nil || mutableInputs {
		t.Fatal("worker input runtime permission must remain immutable", err)
	}
	uploader, consumer := subjectSession(t, owner), subjectSession(t, owner)
	storage := subjectStorage(t)
	asset, key, original := subjectStoredPNG(t, owner, uploader, storage)
	teamUploader, actor := subjectTeam(t, owner, uploader, consumer)
	project := subjectProject(t, owner, actor)
	version, release := seedActualSubjectCatalog(t, owner, project)
	saved, err := subject.NewService(subjectwire.NewStore(runtime, storage.objects), time.Now).Save(t.Context(), teamUploader, uuid.Nil, uuid.New(), subject.SaveInput{Name: "真实绑定主体", Description: "跨个人来源的团队正式主体", Primary: subject.MediaSelection{AssetID: asset}})
	if err != nil {
		t.Fatal("save actual global subject", err)
	}
	var source promptdomain.ReferenceSource
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		var known bool
		var err error
		source, known, err = subjectwire.NewPromptSources(tx).ResolveSubject(t.Context(), actor, project, saved.ID.String(), saved.VersionID.String())
		if err == nil && !known {
			t.Fatal("new subject was misclassified as legacy")
		}
		return err
	}); err != nil {
		t.Fatal("resolve actual fixed subject source", err)
	}
	modelVersion, mode := version.String(), "image2image"
	draft := promptdomain.Draft{SchemaVersion: 1, ModelVersion: &modelVersion, Mode: &mode, Params: map[string]any{},
		Document:   promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph", Content: []promptdomain.Inline{{Type: "text", Text: "使用"}, {Type: "reference", Attrs: &promptdomain.ReferenceAttrs{ReferenceID: "subject-ref"}}}}}},
		References: []promptdomain.Reference{{ID: "subject-ref", BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 0, Source: source}},
		LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 1}, Allocations: map[string]promptdomain.LabelAllocation{"subject-ref": {LabelKind: "subject", Ordinal: 1}}},
	}
	drafts := actualSubjectDraftStore(runtime)
	snapshot, err := drafts.SaveDraft(t.Context(), actor, prompt.DraftWrite{ProjectID: project, Key: uuid.New(), Draft: draft})
	if err != nil {
		t.Fatal("save real subject draft", err)
	}
	readback, err := drafts.ReadDraft(t.Context(), actor, project, snapshot.DraftID)
	if err != nil || readback.Draft.References[0].Source.Media[0].BindingID != saved.Primary.BindingID.String() {
		t.Fatal("draft lost immutable binding", err)
	}
	store := actualSubjectOperationStore(runtime, storage.objects)
	epoch := int64(0)
	quote, err := store.CreateFreeQuote(t.Context(), actor, operation.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operation.FreeQuoteItemInput{InputSource: &promptdomain.InputSource{Kind: "prompt_draft", DraftID: snapshot.DraftID, Revision: snapshot.Revision}, ExpectedSubmissionEpoch: &epoch}})
	if err != nil {
		t.Fatal("real subject draft quote", err)
	}
	if quote.ConfirmKey == nil || quote.SubmissionID == nil || quote.QuoteMicros != 1 || !strings.Contains(quote.FinalPrompt, "主体1") {
		t.Fatal("incomplete quote/editor receipt")
	}
	inputs, err := store.FindOperationInputs(t.Context(), actor, project, quote.OperationID)
	if err != nil || len(inputs) != 2 {
		t.Fatal("load real frozen operation inputs", len(inputs), err)
	}
	in := inputs[1]
	if in.RefType != "subject_media" || in.Role != "reference_image" || in.RefID == nil || *in.RefID != saved.Primary.BindingID || in.MediaAssetID == nil || *in.MediaAssetID != asset || in.RefVersion == nil {
		t.Fatal("semantic subject did not compile to exact reference_image binding", in)
	}
	var frozen struct{ ReleaseID uuid.UUID }
	if err := runtime.Raw(`SELECT release_id FROM operation.provider_binding WHERE operation_id=?`, quote.OperationID).Scan(&frozen).Error; err != nil || frozen.ReleaseID != release {
		t.Fatal("quote did not freeze real managed release", err)
	}
	return actualSubjectGeneration{runtime: runtime, owner: owner, uploader: teamUploader, actor: actor, project: project, storage: storage, key: key, original: original, subject: saved, quote: quote, input: in, store: store}
}

func (f actualSubjectGeneration) confirm(t *testing.T) (operation.ConfirmSingleQuoteResult, error) {
	t.Helper()
	return f.store.ConfirmSingleQuote(t.Context(), f.actor, operation.ConfirmSingleQuoteInput{ProjectID: f.project, OperationID: f.quote.OperationID, RequestID: f.quote.ConfirmKey.String()})
}

func TestActualSubjectDraftQuoteConfirmationAndWorkerSigning(t *testing.T) {
	f := newActualSubjectGeneration(t)
	confirmation, err := f.confirm(t)
	if err != nil || confirmation.ReservationID == uuid.Nil {
		t.Fatal("confirm actual subject quote", err)
	}
	var stored struct {
		Status      string
		ConfirmedBy uuid.UUID
	}
	if err := f.runtime.Raw(`SELECT status,confirmed_by FROM operation.operation WHERE id=?`, f.quote.OperationID).Scan(&stored).Error; err != nil || stored.Status != "confirmed" || stored.ConfirmedBy != f.actor.ID {
		t.Fatal("durable operation confirmer", stored.Status, err)
	}
	var events, held int64
	if err := f.runtime.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic='glowloom.operation.confirmed.v1'`, f.project.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatal("confirmation outbox not committed", events, err)
	}
	if err := f.runtime.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=? AND status='held'`, f.quote.OperationID).Scan(&held).Error; err != nil || held != 1 {
		t.Fatal("real reservation not committed", held, err)
	}
	if err := f.store.CheckSubmissionInputs(t.Context(), f.quote.OperationID); err != nil {
		t.Fatal("real worker submission authority", err)
	}
	ref, err := f.store.ManagedSubjectMediaReference(t.Context(), f.quote.OperationID, f.project, f.input.ID)
	if err != nil || ref.MediaKind != "image" || ref.Role != "reference_image" || ref.URL == "" {
		t.Fatal("real worker signed subject input", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ref.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal("worker signed URL GET", err)
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if response.StatusCode != 200 || readErr != nil || closeErr != nil || !bytes.Equal(data, f.original) {
		t.Fatal("worker signed source did not preserve private original", readErr, closeErr)
	}
	f.storage.put(f.key, bytes.Repeat([]byte{'x'}, len(f.original)))
	if grant, err := f.store.ManagedSubjectMediaReference(t.Context(), f.quote.OperationID, f.project, f.input.ID); !errors.Is(err, media.ErrObjectMismatch) || grant.URL != "" {
		t.Fatal("worker signed corrupted original bytes", err)
	}
	f.storage.put(f.key, f.original)
	if err := f.owner.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=(SELECT team_id FROM workspace.creation_scope WHERE id=?) AND user_id=?`, f.actor.ScopeID, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	before := f.storage.count()
	if err := f.store.CheckSubmissionInputs(t.Context(), f.quote.OperationID); !errors.Is(err, operation.ErrExecutionAuthorityRevoked) {
		t.Fatal("worker input checks ignored durable confirmer revocation", err)
	}
	if grant, err := f.store.ManagedSubjectMediaReference(t.Context(), f.quote.OperationID, f.project, f.input.ID); err == nil || grant.URL != "" || f.storage.count() != before {
		t.Fatal("revoked confirmer reached private source signing", err)
	}
}

func TestActualSubjectConfirmationRejectsMissingTamperedAndRevokedBindings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, actualSubjectGeneration)
		want   error
	}{
		{"deleted subject", func(t *testing.T, f actualSubjectGeneration) {
			_, err := subject.NewService(subjectwire.NewStore(f.runtime, f.storage.objects), time.Now).Delete(t.Context(), f.uploader, f.subject.ID, uuid.New(), subject.RevisionInput{ExpectedRevision: f.subject.Revision})
			if err != nil {
				t.Fatal(err)
			}
		}, subject.ErrNotFound},
		{"tampered input proof", func(t *testing.T, f actualSubjectGeneration) {
			if err := f.owner.Exec(`UPDATE operation.operation_input SET ref_version=repeat('a',64) WHERE id=?`, f.input.ID).Error; err != nil {
				t.Fatal(err)
			}
		}, operation.ErrWorkflowInputNotReady},
		{"missing binding identity", func(t *testing.T, f actualSubjectGeneration) {
			if err := f.owner.Exec(`UPDATE operation.operation_input SET ref_id=? WHERE id=?`, uuid.New(), f.input.ID).Error; err != nil {
				t.Fatal(err)
			}
		}, subject.ErrNotFound},
		{"revoked confirmer", func(t *testing.T, f actualSubjectGeneration) {
			if err := f.owner.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=(SELECT team_id FROM workspace.creation_scope WHERE id=?) AND user_id=?`, f.actor.ScopeID, f.actor.ID).Error; err != nil {
				t.Fatal(err)
			}
		}, workspace.ErrScopeNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newActualSubjectGeneration(t)
			test.change(t, f)
			if result, err := f.confirm(t); !errors.Is(err, test.want) || result.ReservationID != uuid.Nil {
				t.Fatal("invalid binding was financially confirmed", err)
			}
			var reservations int64
			if err := f.runtime.Raw(`SELECT count(*) FROM billing.reservation WHERE operation_id=?`, f.quote.OperationID).Scan(&reservations).Error; err != nil || reservations != 0 {
				t.Fatal("rejected confirmation retained reservation", reservations, err)
			}
			var status string
			if err := f.runtime.Raw(`SELECT status FROM operation.operation WHERE id=?`, f.quote.OperationID).Scan(&status).Error; err != nil || status != "quoted" {
				t.Fatal("rejected confirmation advanced operation", status, err)
			}
		})
	}
}
