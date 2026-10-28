package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

type subjectQuoteOwner struct {
	fact           operationapp.SubjectMediaFact
	actor, project uuid.UUID
	revoked        bool
	verified       int
}

func (o *subjectQuoteOwner) ResolveQuote(_ context.Context, actor identity.Principal, project, binding uuid.UUID) (operationapp.SubjectMediaFact, error) {
	if o.revoked || actor.ID != o.actor || project != o.project || binding != o.fact.BindingID {
		return operationapp.SubjectMediaFact{}, operationapp.ErrFreeQuoteInputNotReady
	}
	return o.fact, nil
}
func (o *subjectQuoteOwner) VerifyInput(_ context.Context, actor identity.Principal, _ uuid.UUID, project uuid.UUID, input domain.OperationInput) error {
	o.verified++
	if o.revoked || actor.ID != o.actor || project != o.project || input.RefID == nil || *input.RefID != o.fact.BindingID || input.RefVersion == nil || *input.RefVersion != o.fact.BindingSHA256 {
		return operationapp.ErrWorkflowInputNotReady
	}
	return nil
}
func (*subjectQuoteOwner) SignedInputURL(context.Context, uuid.UUID, uuid.UUID, domain.OperationInput, time.Duration) (operationapp.ProviderReference, error) {
	return operationapp.ProviderReference{}, operationapp.ErrWorkflowInputNotReady
}

func TestSubjectQuoteRequiresProofAndRechecksRevocationAtConfirm(t *testing.T) {
	db := operationStoreDB(t)
	actor, project := operationStoreProject(t, db)
	_, sourceProject := operationStoreProject(t, db)
	quoted, model, _ := seedQuoteCatalog(t, db, project, time.Now().UTC())
	if err := db.Exec(`UPDATE catalog.capability SET input_roles=ARRAY['prompt','subject'] WHERE key=?`, quoted.Capability).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE catalog.model_profile_version SET limits='{"max_outputs":1,"roles":{"subject":{"max_count":2,"types":["image"]}}}'::jsonb WHERE id=?`, quoted.ModelProfileVersionID).Error; err != nil {
		t.Fatal(err)
	}
	asset, binding := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,'image/png',50,?,'passed')`, asset, sourceProject, "synthetic/subject-"+asset.String(), strings.Repeat("a", 64)).Error; err != nil {
		t.Fatal(err)
	}
	input := operationapp.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: operationapp.FreeQuoteItemInput{ModelKey: "quote-model-" + model.String(), Capability: quoted.Capability, Mode: "text_to_image", Prompt: "主体", Params: json.RawMessage(`{}`), OutputCount: 1, MediaInputs: []operationapp.FreeQuoteMediaInput{{Role: "subject", MediaAssetID: asset}}}}
	store := operationStore(db)
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("bare foreign asset upgraded by role: %v", err)
	}
	input.RequestID = uuid.NewString()
	input.MediaInputs[0].SubjectBindingID = &binding
	if _, err := store.CreateFreeQuote(t.Context(), actor, input); !errors.Is(err, operationapp.ErrFreeQuoteInputNotReady) {
		t.Fatalf("binding without owner accepted: %v", err)
	}
	owner := &subjectQuoteOwner{actor: actor.ID, project: project, fact: operationapp.SubjectMediaFact{BindingID: binding, BindingSHA256: strings.Repeat("b", 64), AssetID: asset, AssetRevision: 1, Kind: "image", SHA256: strings.Repeat("a", 64), ByteSize: 50}}
	store.WithSubjectMedia(func(*gorm.DB) operationapp.SubjectMediaAccess { return owner })
	input.RequestID = uuid.NewString()
	result, err := store.CreateFreeQuote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := store.FindOperationInputs(t.Context(), actor, project, result.OperationID)
	if err != nil || len(inputs) != 2 || inputs[1].RefType != "subject_media" || inputs[1].RefID == nil || *inputs[1].RefID != binding {
		t.Fatalf("immutable binding not persisted: %+v, %v", inputs, err)
	}
	owner.revoked = true
	confirm := operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: result.OperationID, RequestID: uuid.NewString()}
	if _, err = store.ConfirmSingleQuote(t.Context(), actor, confirm); !errors.Is(err, operationapp.ErrWorkflowInputNotReady) {
		t.Fatalf("revoked binding confirmed: %v", err)
	}
	owner.revoked = false
	confirm.RequestID = uuid.NewString()
	if _, err = store.ConfirmSingleQuote(t.Context(), actor, confirm); err != nil {
		t.Fatal(err)
	}
	if owner.verified < 2 {
		t.Fatalf("binding not rechecked for each confirmation: %d", owner.verified)
	}
}
