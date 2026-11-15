package microservices_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpcpinproof"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/mediapins"
	creationpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	creationapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/creationpinproof"
	mediarpc "github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcreference"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func consumptionKey(pin mediadomain.ReferencePin) creationapp.ReferencePinConsumptionKey {
	i := pin.Identity
	return creationapp.ReferencePinConsumptionKey{PinID: i.PinID, AcquireCommandID: pin.AcquireCommandID, SourceCommandID: i.ConsumerCommandID,
		SourceCommandSHA256: i.ConsumerCommandSHA256, HandoffID: i.ConsumerResourceID, ScopeID: i.ScopeID, ProjectID: i.ProjectID,
		AssetID: i.AssetID, AssetRevision: i.AssetRevision, Kind: pin.Kind, PinSnapshotSHA256: pin.SnapshotSHA256()}
}

func creationStoreWithPin(t *testing.T, f *frozenFixture, pin mediadomain.ReferencePin) *creationpg.FrozenInputStore {
	t.Helper()
	if len(f.request.Pins) != 1 || f.request.Pins[0].PinID != pin.Identity.PinID {
		t.Fatal("fixture pin identity mismatch")
	}
	serverTransport, clientTransport, _ := ownerPeerTLS(t, "creation")
	server, err := mediarpc.NewServer(mediapg.NewReferencePinStore(f.media, nil, nil), mediaReceiptToken, "creation", serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mediapins.New(mediapins.Target{Address: serveFrozenRPC(t, server), Token: mediaReceiptToken, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return creationpg.NewFrozenInputStore(f.creation, client)
}

func pinConsumerClient(t *testing.T, f *frozenFixture) *creationpinproof.Client {
	t.Helper()
	serverTransport, clientTransport, _ := ownerPeerTLS(t, "media")
	token := strings.Repeat("media-source-proof-fixture", 2)
	server, err := grpcpinproof.NewServer(f.store, token, "media", serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client, err := creationpinproof.New(creationpinproof.Target{Address: serveFrozenRPC(t, server), Token: token, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func pinCustodyCommand(f *frozenFixture, pin mediadomain.ReferencePin, action string) mediaapp.ReferencePinRequest {
	// Custody bookkeeping requires live resource fences and permanent consumer
	// evidence; it does not mint another user-session admission.
	var authorities []writefence.Authority
	for _, authority := range f.request.Command.Authorities {
		if authority.Issuer == writefence.Creation {
			authorities = append(authorities, authority)
		}
	}
	return mediaapp.ReferencePinRequest{Command: writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("e", 64), Action: "media.reference_pin." + action, Authorities: authorities}, Identity: pin.Identity}
}

func TestMediaReferenceConsumptionAdoptedAndBlockedHistoryCannotRelease(t *testing.T) {
	f, _, pin := frozenPinReceiptFixture(t)
	key := consumptionKey(pin)
	if proof, err := f.store.ReadReferencePinConsumption(t.Context(), key); err == nil || proof.Outcome != "" {
		t.Fatal("missing snapshot with admitted command became non-adoption proof", proof, err)
	}
	// Both receipt directions use real restricted stores and authenticated TCP RPCs.
	f.store = creationStoreWithPin(t, f, pin)
	envelope := f.freeze(t)
	proof, err := f.store.ReadReferencePinConsumption(t.Context(), key)
	if err != nil || proof.Outcome != "adopted" || proof.SourceSnapshotSHA256 != envelope.SnapshotSHA256 {
		t.Fatal("committed source did not prove its exact pin", proof, err)
	}
	client := pinConsumerClient(t, f)
	store := mediapg.NewReferencePinStore(f.media, client, nil)
	request := pinCustodyCommand(f, pin, "bind")
	bound, err := store.Bind(t.Context(), request)
	if err != nil || bound.State != "bound" || bound.AdoptionReceiptSHA256 == "" {
		t.Fatal("actual source RPC proof did not bind Media custody", bound, err)
	}
	cancel := f.cancelCommand(t)
	if _, err := f.store.BlockFrozenInput(t.Context(), envelope.Key(), cancel); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(t.Context(), pinCustodyCommand(f, pin, "release")); !errors.Is(err, mediadomain.ErrReferencePinProof) {
		t.Fatal("source cancellation erased retained historical reference", err)
	}
	replay, err := store.Bind(t.Context(), request)
	if err != nil || !reflect.DeepEqual(bound, replay) {
		t.Fatal("original bind acknowledgement changed", replay, err)
	}
}

func TestMediaReferenceConsumptionPermanentFreezeReleasesOrphanOnly(t *testing.T) {
	f, originalStore, pin := frozenPinReceiptFixture(t)
	client := pinConsumerClient(t, f)
	store := mediapg.NewReferencePinStore(f.media, client, nil)
	request := pinCustodyCommand(f, pin, "release")
	if _, err := store.Release(t.Context(), request); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("pending source or 404 became a release proof", err)
	}
	if current, err := originalStore.ReadReferencePinReceipt(t.Context(), pinReceiptKey(pin)); err != nil || current.State != "held" {
		t.Fatal("unknown consumer outcome released the held pin", current, err)
	}
	if _, err := pgfence.New(f.creation).Freeze(t.Context(), f.request.SourceCommand.ID); err != nil {
		t.Fatal(err)
	}
	// Account revocation cannot strand proved non-adoption or reopen user access.
	coordinator := pgfence.NewAuthorityStore(f.identity)
	closing, err := coordinator.Begin(t.Context(), f.request.Command.Authorities[0], uuid.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	writers := map[writefence.Writer]*pgfence.Store{
		writefence.WriterIdentity: pgfence.New(f.identity), writefence.WriterCreation: pgfence.New(f.creation),
		writefence.WriterMedia: pgfence.New(f.media), writefence.WriterExecution: pgfence.New(f.execution),
	}
	for _, writer := range writefence.Writers() {
		if err := writers[writer].Install(t.Context(), closing.Control); err != nil {
			t.Fatal(err)
		}
		closing, err = coordinator.Acknowledge(t.Context(), closing.Control, writer)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !closing.Complete {
		t.Fatal("account closure did not retain four exact acknowledgements")
	}
	released, err := store.Release(t.Context(), request)
	if err != nil || released.State != "released" || released.ReleaseOutcome != "not_adopted_frozen" || released.ReleaseReceiptSHA256 == "" {
		t.Fatal("original permanently frozen source did not release orphan", released, err)
	}
	newUserCommand := f.request.Command
	newUserCommand.ID, newUserCommand.Action = uuid.New(), "media.reference_pin.acquire"
	if _, err := pgfence.New(f.media).Admit(t.Context(), newUserCommand); !errors.Is(err, writefence.ErrFenced) {
		t.Fatal("custody completion reopened a revoked account permit", err)
	}
	if _, err := f.store.FreezeInput(t.Context(), f.request); !errors.Is(err, writefence.ErrFrozen) {
		t.Fatal("released orphan was adopted by a late source commit", err)
	}
	if proof, err := f.store.ReadReferencePinConsumption(t.Context(), consumptionKey(pin)); err != nil || proof.Outcome != "not_adopted_frozen" || proof.SourceSnapshotSHA256 != "" {
		t.Fatal("terminal source proof not durable", proof, err)
	}
}

func TestMediaReferenceConsumptionRejectsSubstitutedPinFacts(t *testing.T) {
	f, _, pin := frozenPinReceiptFixture(t)
	f.store = creationStoreWithPin(t, f, pin)
	f.freeze(t)
	for _, field := range []string{"pin", "acquire", "hash", "scope", "project", "asset", "revision", "kind"} {
		t.Run(field, func(t *testing.T) {
			key := consumptionKey(pin)
			switch field {
			case "pin":
				key.PinID = uuid.New()
			case "acquire":
				key.AcquireCommandID = uuid.New()
			case "hash":
				key.PinSnapshotSHA256 = strings.Repeat("f", 64)
			case "scope":
				key.ScopeID = uuid.New()
			case "project":
				key.ProjectID = uuid.New()
			case "asset":
				key.AssetID = uuid.New()
			case "revision":
				key.AssetRevision++
			case "kind":
				key.Kind = "video"
			}
			if _, err := f.store.ReadReferencePinConsumption(t.Context(), key); !errors.Is(err, promptdomain.ErrFrozenInputConflict) {
				t.Fatal("substituted pin gained a source proof", err)
			}
		})
	}
}
