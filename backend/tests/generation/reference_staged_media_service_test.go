package generation_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	"github.com/google/uuid"
)

type stagedMediaMemory struct {
	app.ReferenceCallDispatchRepository
	actor                        app.Actor
	receipt                      domain.ReferenceCallReceipt
	target                       app.ReferenceGenerationTarget
	targetRef                    domain.GenerationRevisionRef
	media                        domain.ReferenceStagedMedia
	inside, deny, failCompletion bool
	reads, updates               int
	data                         []byte
	readErr                      error
	afterRead                    func()
}

func (m *stagedMediaMemory) WithinReferenceStagedMedia(ctx context.Context, f func(app.ReferenceStagedMediaRepository) error) error {
	before := m.media
	m.inside = true
	err := f(m)
	if err == nil && m.failCompletion && m.media.Revision == 2 {
		err = errors.New("injected completion commit failure")
	}
	if err != nil {
		m.media = before
	}
	m.inside = false
	return err
}
func (m *stagedMediaMemory) LockProviderWorkspace(_ context.Context, workspace string) error {
	if workspace != m.receipt.WorkspaceID {
		return errors.New("foreign workspace")
	}
	return nil
}
func (m *stagedMediaMemory) AuthorizeReferenceGenerationProject(_ context.Context, actor app.Actor, workspace, project string) error {
	if m.deny || actor != m.actor || workspace != m.receipt.WorkspaceID || project != m.receipt.ProjectID {
		return errors.New("denied")
	}
	return nil
}
func (m *stagedMediaMemory) FindReferenceExecution(_ context.Context, _, _, id string) (domain.ReferenceExecution, error) {
	r := m.receipt.Call.ExecutionRef
	if id != r.ID {
		return domain.ReferenceExecution{}, errors.New("foreign execution")
	}
	return domain.ReferenceExecution{Revision: r.Revision, ContentHash: r.ContentHash, InitialReferenceExecutionInput: domain.InitialReferenceExecutionInput{ID: r.ID, ReadSet: domain.ReferenceExecutionReadSet{TargetRef: m.targetRef}}}, nil
}
func (m *stagedMediaMemory) FindReferenceProviderJob(context.Context, string, string, string) (domain.ReferenceProviderJob, []domain.ReferenceProviderCall, error) {
	return domain.BuildReferenceProviderJob(m.receipt.Call.ExecutionRef, []domain.ReferenceProviderCallInput{m.receipt.Call.ReferenceProviderCallInput})
}
func (m *stagedMediaMemory) FindReferenceCallState(_ context.Context, _, _, _, key string) (domain.ReferenceCallState, error) {
	if key != m.receipt.Call.CallKey {
		return domain.ReferenceCallState{}, errors.New("foreign call")
	}
	return domain.ReferenceCallState{Status: domain.ProviderCallSucceeded, CallKey: key, Receipt: &m.receipt}, nil
}
func (m *stagedMediaMemory) FindReferenceGenerationTarget(context.Context, string, string, string) (app.ReferenceGenerationTarget, error) {
	return m.target, nil
}
func (m *stagedMediaMemory) FindReferenceStagedMedia(context.Context, string, string, string) (domain.ReferenceStagedMedia, error) {
	if m.media.ID == "" {
		return domain.ReferenceStagedMedia{}, app.ErrReferenceStagedMediaNotFound
	}
	return m.media, nil
}
func (m *stagedMediaMemory) InsertReferenceStagedMedia(_ context.Context, value domain.ReferenceStagedMedia) error {
	if m.media.ID != "" {
		return errors.New("duplicate media")
	}
	m.media = value
	return nil
}
func (m *stagedMediaMemory) UpdateReferenceStagedMedia(_ context.Context, before, after domain.ReferenceStagedMedia) error {
	if !reflect.DeepEqual(before, m.media) {
		return errors.New("CAS conflict")
	}
	if err := domain.ValidateReferenceStagedMediaTransition(before, after); err != nil {
		return err
	}
	m.updates++
	m.media = after
	return nil
}
func (m *stagedMediaMemory) ReadVerified(_ context.Context, key string, size int64, hash string, max int64) ([]byte, error) {
	m.reads++
	if m.inside || m.media.State != "quarantined" || key != m.receipt.Output.StagingObjectKey || size != m.receipt.Output.Bytes || hash != m.receipt.Output.SHA256 || max != size {
		return nil, errors.New("read violated committed object boundary")
	}
	if m.afterRead != nil {
		m.afterRead()
	}
	return bytes.Clone(m.data), m.readErr
}

func TestReferenceStagedMediaRecoversWithoutProviderOrAssetPublication(t *testing.T) {
	for _, mode := range []string{"success", "unavailable", "checksum", "size", "commit", "revoked", "cancelled", "foreign_receipt", "foreign_target", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			receipt, data, location := referenceStagedFixture(t)
			actor := app.Actor{UserID: uuid.NewString(), TokenVersion: 1}
			// This fixture isolates transaction/IO behavior. The full workflow journey
			// separately proves accepted Target/Execution/Receipt facts in PostgreSQL.
			m := &stagedMediaMemory{actor: actor, receipt: receipt, data: data}
			m.targetRef = domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
			m.target = app.ReferenceGenerationTarget{ID: m.targetRef.ID, Revision: m.targetRef.Revision, ContentHash: m.targetRef.ContentHash, OutputContract: domain.ReferenceOutputContract{Slots: []domain.ReferenceOutputSlot{receipt.Slot}}}
			location.ObjectKey = ""
			service, err := app.NewReferenceStagedMediaService(m, m, location, func() time.Time { return receipt.ObservedAt.Add(time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			command := app.MaterializeReferenceStagedMediaCommand{WorkspaceID: receipt.WorkspaceID, ProjectID: receipt.ProjectID, ExecutionRef: receipt.Call.ExecutionRef, CallKey: receipt.Call.CallKey, ReceiptRef: domain.GenerationActionRef{ID: receipt.SubmissionToken, ContentHash: receipt.ContentHash}}
			switch mode {
			case "unavailable":
				m.readErr = errors.New("private-object-url must not leak")
			case "checksum":
				m.readErr = objectstore.ErrObjectChecksumMismatch
			case "size":
				m.readErr = objectstore.ErrObjectSizeMismatch
			case "commit":
				m.failCompletion = true
			case "revoked":
				m.afterRead = func() { m.deny = true }
			case "cancelled":
				m.afterRead = cancel
			case "foreign_receipt":
				command.ReceiptRef.ContentHash = strings.Repeat("f", 64)
			case "foreign_target":
				m.target.ContentHash = strings.Repeat("f", 64)
			case "corrupt":
				m.media, _ = domain.NewReferenceStagedMedia(receipt, domain.ReferenceObjectStoreRef{Profile: location.Profile, Bucket: location.Bucket, ObjectKey: receipt.Output.StagingObjectKey})
				m.media.SHA256 = strings.Repeat("f", 64)
			}
			result, err := service.Materialize(ctx, actor, command)
			switch mode {
			case "success", "checksum", "size":
				expected := "ready_for_review"
				if mode != "success" {
					expected = "rejected"
				}
				if err != nil || result.State != expected || m.reads != 1 || m.updates != 1 {
					t.Fatalf("materialize: %+v err=%v reads=%d updates=%d", result, err, m.reads, m.updates)
				}
				replay, err := service.Materialize(context.Background(), actor, command)
				if err != nil || !reflect.DeepEqual(replay, result) || m.reads != 1 {
					t.Fatalf("replay repeated IO: %v", err)
				}
			case "unavailable", "commit", "revoked", "cancelled":
				if err == nil || result.ID != "" || m.media.State != "quarantined" || strings.Contains(err.Error(), "private-object-url") {
					t.Fatalf("failure published media or leaked object: %+v %v", result, err)
				}
				m.readErr, m.afterRead, m.deny, m.failCompletion = nil, nil, false, false
				if mode == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation identity lost: %v", err)
				}
				result, err = service.Materialize(context.Background(), actor, command)
				if err != nil || result.State != "ready_for_review" || result.ID != receipt.SubmissionToken {
					t.Fatalf("recovery lost media identity: %+v %v", result, err)
				}
			default:
				if err == nil || result.ID != "" || m.reads != 0 {
					t.Fatalf("invalid provenance reached IO: %+v %v", result, err)
				}
			}
		})
	}
}
