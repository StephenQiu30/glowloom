package microservices_test

import (
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type scopeBootstrapWriters struct {
	stores       map[writefence.Writer]*pgfence.Store
	calls        map[writefence.Writer]int
	lostMediaACK bool
}

func (w *scopeBootstrapWriters) Install(ctx context.Context, writer writefence.Writer, control writefence.Control) error {
	w.calls[writer]++
	if err := w.stores[writer].Install(ctx, control); err != nil {
		return err
	}
	if writer == writefence.WriterMedia && w.lostMediaACK {
		w.lostMediaACK = false
		return errors.New("synthetic lost ACK after durable Media install")
	}
	return nil
}

func TestCreationScopeBootstrapPreparesRealResourcesAndRecoversOriginalFourWriterControl(t *testing.T) {
	admin := fenceDatabase(t, "")
	creator := authorityDatabase(t, "creation")
	writers := &scopeBootstrapWriters{stores: map[writefence.Writer]*pgfence.Store{}, calls: map[writefence.Writer]int{}, lostMediaACK: true}
	for _, writer := range writefence.Writers() {
		writers.stores[writer] = pgfence.New(authorityDatabase(t, string(writer)))
	}
	registration := workspacesource.PersonalRegistration{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: strings.Repeat("a", 64)}
	resources, err := workspacesource.NewPersonalRegistrations(pgsource.NewPersonalRegistrationStore(creator))
	if err != nil {
		t.Fatal(err)
	}
	source := pgsource.New(creator)
	ready, err := workspacesource.NewReadyPersonalRegistrations(resources, source, writefence.NewCoordinator(pgfence.NewAuthorityStore(creator), writers))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ready.Prepare(t.Context(), registration); err == nil {
		t.Fatal("partial writer install reported the personal scope ready")
	}
	initial := workspacesource.InitialScopeControl(registration)
	if err := source.CheckPersonalScopeAuthority(t.Context(), registration, initial); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("pending scope granted source readiness: %v", err)
	}
	var physical struct{ ScopeID, PhysicalScopeID, OwnerUserID uuid.UUID }
	if err := admin.Raw(`SELECT id AS scope_id,physical_scope_id,owner_user_id FROM workspace.creation_scope WHERE id=?`, registration.ScopeID).Scan(&physical).Error; err != nil {
		t.Fatal(err)
	}
	if physical.ScopeID != registration.ScopeID || physical.PhysicalScopeID != registration.OrganizationID || physical.OwnerUserID != registration.AccountID {
		t.Fatal("original accepted registration did not prepare its real scope")
	}
	// Reconstruct the use case and coordinator; no caller-selected replacement ID.
	restarted, err := workspacesource.NewReadyPersonalRegistrations(resources, pgsource.New(creator), writefence.NewCoordinator(pgfence.NewAuthorityStore(creator), writers))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := restarted.Resolve(t.Context(), registration)
	if err != nil || receipt != registration {
		t.Fatalf("original source preparation did not recover: %+v %v", receipt, err)
	}
	if err := source.CheckPersonalScopeAuthority(t.Context(), registration, initial); err != nil {
		t.Fatal(err)
	}
	for _, writer := range writefence.Writers() {
		want := 1
		if writer == writefence.WriterMedia {
			want = 2
		}
		if writers.calls[writer] != want {
			t.Fatalf("writer %s installed %d times; want %d", writer, writers.calls[writer], want)
		}
	}
	if _, err := restarted.Prepare(t.Context(), registration); err != nil {
		t.Fatal("exact receipt replay", err)
	}
	for _, writer := range writefence.Writers() {
		want := 1
		if writer == writefence.WriterMedia {
			want = 2
		}
		if writers.calls[writer] != want {
			t.Fatalf("ready replay reinstalled %s", writer)
		}
	}
	changed := registration
	changed.AccountID = uuid.New()
	if _, err := restarted.Prepare(t.Context(), changed); !errors.Is(err, workspacesource.ErrPersonalRegistration) {
		t.Fatalf("original registration adopted another account: %v", err)
	}
	blocked, err := writefence.NewCoordinator(pgfence.NewAuthorityStore(creator), writers).Set(t.Context(), initial.Authority, uuid.New(), true)
	if err != nil || !blocked.Complete {
		t.Fatalf("scope closure: %+v %v", blocked, err)
	}
	if _, err := restarted.Resolve(t.Context(), registration); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("initial preparation reopened a closed scope: %v", err)
	}
	var head struct {
		Epoch             int64
		Blocked, Complete bool
		CoordinationID    uuid.UUID
	}
	if err := admin.Raw(`SELECT epoch,blocked,complete,coordination_id FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='scope' AND authority_id=?`, registration.ScopeID).Scan(&head).Error; err != nil {
		t.Fatal(err)
	}
	if head.Epoch != 2 || !head.Blocked || !head.Complete || head.CoordinationID != blocked.Control.CoordinationID {
		t.Fatal("historical initial-open receipt replaced the current closed head")
	}
}

func TestCreationScopeBootstrapRejectsUnacceptedResourcesAndForeignRole(t *testing.T) {
	admin := fenceDatabase(t, "")
	creator := authorityDatabase(t, "creation")
	registration := workspacesource.PersonalRegistration{OperationID: uuid.New(), AccountID: uuid.New(), OrganizationID: uuid.New(), ScopeID: uuid.New(), InputSHA256: strings.Repeat("b", 64)}
	if _, err := pgsource.New(creator).PreparePersonalScopeAuthority(t.Context(), registration); !errors.Is(err, workspacesource.ErrPersonalRegistration) {
		t.Fatalf("unaccepted resource initialized an authority: %v", err)
	}
	if _, err := pgsource.New(authorityDatabase(t, "identity")).PreparePersonalScopeAuthority(t.Context(), registration); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("Identity initialized Creation authority: %v", err)
	}
	var count int
	if err := admin.Raw(`SELECT count(*) FROM infra.authority_head WHERE authority_id=?`, registration.ScopeID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("unaccepted scope left a head: %d %v", count, err)
	}
}
