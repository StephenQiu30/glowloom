package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func mediaWorkRequest(t *testing.T, f pinFixture) mediaapp.ProjectWorkRequest {
	t.Helper()
	var project struct{ OrgID uuid.UUID }
	if err := f.admin.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.acquire.Identity.ProjectID).Scan(&project).Error; err != nil {
		t.Fatal(err)
	}
	return mediaapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: f.acquire.Identity.ProjectID, OrganizationID: project.OrgID, ScopeID: f.acquire.Identity.ScopeID, ProjectEpoch: 1, ManifestSHA256: strings.Repeat("e", 64)}
}

func TestMediaProjectWorkUnboundPinIsNotIdle(t *testing.T) {
	f := newPinFixture(t)
	pin, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
	if err != nil {
		t.Fatal(err)
	}
	request := mediaWorkRequest(t, f)
	value, err := mediapg.NewProjectWorkReader(f.media).ReadProjectWork(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if value.BusinessState != mediaapp.ProjectWorkUnknown {
		t.Fatalf("held pin without final consumer decision was declared %s", value.BusinessState)
	}
	var state string
	if err := f.admin.Raw(`SELECT state FROM media.reference_pin WHERE pin_id=?`, pin.Identity.PinID).Scan(&state).Error; err != nil || state != "held" {
		t.Fatalf("read changed pin: %s %v", state, err)
	}
	// The lifecycle verifier here isolates Media's read projection; the actual
	// bidirectional consumer proof is accepted by the separate pin RPC tests.
	verifier := pinVerifier(func(_ context.Context, original mediadomain.ReferencePin, _ bool) (mediadomain.ReferencePinProof, error) {
		return pinProof(original, "adopted"), nil
	})
	if _, err := mediapg.NewReferencePinStore(f.media, verifier, nil).Bind(t.Context(), f.request("bind")); err != nil {
		t.Fatal(err)
	}
	request.Commands = []writefence.Command{f.acquire.Command}
	value, err = mediapg.NewProjectWorkReader(f.media).ReadProjectWork(t.Context(), request)
	if err != nil || value.BusinessState != mediaapp.ProjectWorkIdle || value.Commands[0].State != "committed" {
		t.Fatalf("historical bound pin remained occupied: %+v %v", value, err)
	}
}

func TestMediaProjectWorkReadsOriginalCommandsWithoutAdmittingMissing(t *testing.T) {
	f := newPinFixture(t)
	request := mediaWorkRequest(t, f)
	request.Commands = []writefence.Command{f.acquire.Command}
	reader := mediapg.NewProjectWorkReader(f.media)
	value, err := reader.ReadProjectWork(t.Context(), request)
	if err != nil || len(value.Commands) != 1 || value.Commands[0].State != "unknown" {
		t.Fatalf("missing evidence: %+v %v", value, err)
	}
	if _, err := pgfence.New(f.media).Load(t.Context(), f.acquire.Command.ID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("reader admitted missing command: %v", err)
	}
	if _, err := pgfence.New(f.media).Admit(t.Context(), f.acquire.Command); err != nil {
		t.Fatal(err)
	}
	value, err = reader.ReadProjectWork(t.Context(), request)
	if err != nil || value.Commands[0].State != "admitted" || value.BusinessState != mediaapp.ProjectWorkBusy {
		t.Fatalf("retained admission: %+v %v", value, err)
	}
	if _, err := pgfence.New(f.media).Freeze(t.Context(), f.acquire.Command.ID); err != nil {
		t.Fatal(err)
	}
	value, err = reader.ReadProjectWork(t.Context(), request)
	if err != nil || value.Commands[0].State != "frozen" || value.BusinessState != mediaapp.ProjectWorkIdle {
		t.Fatalf("permanent freeze: %+v %v", value, err)
	}
	request.Commands[0].SHA256 = strings.Repeat("f", 64)
	if _, err := reader.ReadProjectWork(t.Context(), request); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("substituted command returned evidence: %v", err)
	}
}

func TestMediaProjectWorkCommittedMarkerWithoutBusinessReceiptIsUnknown(t *testing.T) {
	f := newPinFixture(t)
	if _, err := pgfence.New(f.media).Admit(t.Context(), f.acquire.Command); err != nil {
		t.Fatal(err)
	}
	if err := f.media.Transaction(func(tx *gorm.DB) error {
		store := pgfence.New(tx)
		if err := store.Guard(t.Context(), tx, f.acquire.Command); err != nil {
			return err
		}
		return store.Complete(t.Context(), tx, f.acquire.Command, []byte(`{"marker":"technical-only"}`))
	}); err != nil {
		t.Fatal(err)
	}
	request := mediaWorkRequest(t, f)
	request.Commands = []writefence.Command{f.acquire.Command}
	value, err := mediapg.NewProjectWorkReader(f.media).ReadProjectWork(t.Context(), request)
	if err != nil || value.BusinessState != mediaapp.ProjectWorkUnknown || len(value.Commands) != 1 || value.Commands[0].State != "committed" {
		t.Fatalf("technical marker replaced a business receipt: %+v %v", value, err)
	}
}

func TestMediaProjectWorkEmptyManifestStillChecksActualProcessing(t *testing.T) {
	f := newPinFixture(t)
	if err := f.media.Exec(`UPDATE media.media_asset SET status='processing' WHERE id=?`, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	value, err := mediapg.NewProjectWorkReader(f.media).ReadProjectWork(t.Context(), mediaWorkRequest(t, f))
	if err != nil || value.BusinessState != mediaapp.ProjectWorkBusy || len(value.Commands) != 0 {
		t.Fatalf("empty manifest hid processing: %+v %v", value, err)
	}
}

func TestMediaProjectWorkRequiresActualMediaAndIndependentPool(t *testing.T) {
	f := newPinFixture(t)
	request := mediaWorkRequest(t, f)
	for _, owner := range []string{"identity", "creation", "execution"} {
		if _, err := mediapg.NewProjectWorkReader(authorityDatabase(t, owner)).ReadProjectWork(t.Context(), request); !errors.Is(err, writefence.ErrInvalid) {
			t.Fatalf("foreign owner %s: %v", owner, err)
		}
	}
	if err := f.media.Transaction(func(tx *gorm.DB) error {
		if _, err := mediapg.NewProjectWorkReader(tx).ReadProjectWork(t.Context(), request); !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("caller transaction: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
