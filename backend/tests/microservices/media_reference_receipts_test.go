package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/mediapins"
	creationpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	mediarpc "github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcreference"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

var mediaReceiptToken = strings.Repeat("pin-receipt-fixture", 3)

type pinReceiptRead func(context.Context, mediaapp.ReferencePinReceiptKey) (mediadomain.ReferencePin, error)

func (f pinReceiptRead) ReadReferencePinReceipt(ctx context.Context, key mediaapp.ReferencePinReceiptKey) (mediadomain.ReferencePin, error) {
	return f(ctx, key)
}

func frozenPinReceiptFixture(t *testing.T) (*frozenFixture, *mediapg.ReferencePinStore, mediadomain.ReferencePin) {
	t.Helper()
	f := newFrozenFixture(t)
	asset, location, acquireID, pinID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	profile, objectKey := "receipt-"+uuid.NewString(), "reference-receipts/"+asset.String()+".png"
	digest := strings.Repeat("b", 64)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES (?,'cos','https://fixture.invalid','https://fixture.invalid','fixture','receipt-fixture','virtual',?)`, []any{profile, fmt.Sprintf("%x", sha256.Sum256([]byte(profile)))}},
		{`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES (?,?,'receipt-fixture',?,'accepted',12,?,'image/png')`, []any{location, profile, objectKey, digest}},
		{`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status,moderation_detail,object_location_id) VALUES (?,?,'image','upload','ready',?,'image/png',12,?,'passed','{"number":9007199254740993,"scale":1.2300}',?)`, []any{asset, f.request.ProjectID, objectKey, digest, location}},
	} {
		if err := f.admin.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.draft.References = []promptdomain.Reference{{ID: "image-ref", BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: 0, Source: promptdomain.ReferenceSource{Kind: "media", SourceID: asset.String(), SourceVersion: "1"}}}
	f.draft.LabelState.HighWater["image"] = 1
	f.draft.LabelState.Allocations["image-ref"] = promptdomain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	if err := f.draft.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(f.draft)
	if err != nil {
		t.Fatal(err)
	}
	draftHash, inputHash := f.draft.Hashes()
	if err := f.admin.Exec(`UPDATE workspace.prompt_draft SET draft=?::jsonb,draft_hash=?,input_hash=? WHERE id=?`, string(raw), draftHash, inputHash, f.request.Source.DraftID).Error; err != nil {
		t.Fatal(err)
	}
	f.request.HandoffID, f.request.RequestID, f.request.Command.ID = uuid.New(), uuid.New(), uuid.New()
	f.request.Pins = []promptdomain.FrozenPin{{ReferenceID: "image-ref", PinID: pinID, AcquireCommandID: acquireID, AssetID: asset, AssetRevision: 1, Kind: "image"}}
	f.bind(t)
	store := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
	pin, err := store.Acquire(t.Context(), mediaapp.ReferencePinRequest{
		Command: writefence.Command{ID: acquireID, Action: "media.reference_pin.acquire", SHA256: strings.Repeat("c", 64), Authorities: f.request.Command.Authorities},
		Identity: mediadomain.ReferencePinIdentity{PinID: pinID, Consumer: "creation", ConsumerCommandID: f.request.SourceCommand.ID,
			ConsumerCommandSHA256: f.request.SourceCommand.SHA256, ConsumerResourceID: f.request.HandoffID, ScopeID: f.request.ScopeID, ProjectID: f.request.ProjectID,
			AssetID: asset, LocationID: location, ByteSize: 12, SHA256: digest, AssetRevision: 1, GateEpoch: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.request.Pins[0].SnapshotSHA256 = pin.SnapshotSHA256()
	if got, err := f.request.SourceCommandSHA256(); err != nil || got != f.request.SourceCommand.SHA256 {
		t.Fatal("returned Media digest changed the original source command", got, err)
	}
	return f, store, pin
}

func pinReceiptKey(pin mediadomain.ReferencePin) mediaapp.ReferencePinReceiptKey {
	i := pin.Identity
	return mediaapp.ReferencePinReceiptKey{PinID: i.PinID, AcquireCommandID: pin.AcquireCommandID, ConsumerCommandID: i.ConsumerCommandID,
		ConsumerCommandSHA256: i.ConsumerCommandSHA256, ConsumerResourceID: i.ConsumerResourceID, Consumer: i.Consumer, ScopeID: i.ScopeID, ProjectID: i.ProjectID}
}

func TestMediaReferenceReceiptRequiresOriginalConsumerAndMediaOwner(t *testing.T) {
	f, store, original := frozenPinReceiptFixture(t)
	key := pinReceiptKey(original)
	got, err := store.ReadReferencePinReceipt(t.Context(), key)
	if err != nil || got.Kind != "image" || got.SnapshotSHA256() != original.SnapshotSHA256() || got.State != "held" {
		t.Fatal("original immutable acquisition evidence changed", got, err)
	}
	for _, field := range []string{"pin", "acquire", "consumer_command", "consumer_hash", "resource", "scope", "project", "consumer"} {
		t.Run(field, func(t *testing.T) {
			wrong := key
			switch field {
			case "pin":
				wrong.PinID = uuid.New()
			case "acquire":
				wrong.AcquireCommandID = uuid.New()
			case "consumer_command":
				wrong.ConsumerCommandID = uuid.New()
			case "consumer_hash":
				wrong.ConsumerCommandSHA256 = strings.Repeat("d", 64)
			case "resource":
				wrong.ConsumerResourceID = uuid.New()
			case "scope":
				wrong.ScopeID = uuid.New()
			case "project":
				wrong.ProjectID = uuid.New()
			case "consumer":
				wrong.Consumer = "execution"
			}
			if _, err := store.ReadReferencePinReceipt(t.Context(), wrong); err == nil {
				t.Fatal("unrelated key returned an acquisition proof")
			}
		})
	}
	for _, owner := range []string{"identity", "creation", "execution"} {
		if _, err := mediapg.NewReferencePinStore(authorityDatabase(t, owner), nil, nil).ReadReferencePinReceipt(t.Context(), key); !errors.Is(err, mediadomain.ErrReferencePinOwner) {
			t.Fatalf("%s pool read Media evidence: %v", owner, err)
		}
	}
	tx := f.media.Begin()
	defer func() { _ = tx.Rollback().Error }()
	if _, err := mediapg.NewReferencePinStore(tx, nil, nil).ReadReferencePinReceipt(t.Context(), key); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatal("caller transaction accepted by Media receipt reader", err)
	}
}

func TestMediaReferenceReceiptRealRPCAdoptsFrozenImageAndReplaysOffline(t *testing.T) {
	f, store, original := frozenPinReceiptFixture(t)
	serverTransport, clientTransport, wrongPeer := ownerPeerTLS(t, "creation")
	server, err := mediarpc.NewServer(store, mediaReceiptToken, "creation", serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	bad, err := mediapins.New(mediapins.Target{Address: address, Token: mediaReceiptToken, Transport: wrongPeer})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bad.Close() })
	if _, err := bad.VerifyFrozenInputPins(t.Context(), f.request); !errors.Is(err, promptdomain.ErrReferenceUnavailable) {
		t.Fatal("wrong service certificate reached acquisition evidence", err)
	}
	client, err := mediapins.New(mediapins.Target{Address: address, Token: mediaReceiptToken, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	f.store = creationpg.NewFrozenInputStore(f.creation, client)
	envelope := f.freeze(t)
	if len(envelope.Input.Pins) != 1 || envelope.Input.Pins[0].SnapshotSHA256 != original.SnapshotSHA256() || envelope.Input.Pins[0].Kind != "image" {
		t.Fatal("real Media evidence not frozen into source", envelope)
	}
	server.Stop()
	replay, err := creationpg.NewFrozenInputStore(f.creation, client).FreezeInput(t.Context(), f.request)
	if err != nil || !reflect.DeepEqual(envelope, replay) {
		t.Fatal("committed source replay required live Media", fmt.Sprint(err))
	}
}
