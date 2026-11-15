package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/grpcpins"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"

	identitypg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/creationpins"

	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// This fixture proves source/effect boundaries, not public registration bootstrap.
func pinAdmissionIdentity(t *testing.T, f *sourcePolicyFixture) (*identityrpc.Client, *identitysource.Client, identityrpc.Credential) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, session := f.request.ActorID, f.request.SessionID
	raw := sha256.Sum256(session[:])
	legacy := base64.RawURLEncoding.EncodeToString(raw[:])
	tokenHash := sha256.Sum256(raw[:])
	csrf := base64.RawURLEncoding.EncodeToString(sourceProofProtector{}.Digest("session_csrf", session, legacy))
	csrfHash := sha256.Sum256([]byte(csrf))
	if err := f.admin.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=? WHERE id=?`, actor.String()+"@pin-admission.invalid", now.Add(-time.Hour), actor).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,csrf_hash) VALUES (?,?,?,1,?,?,?,false,1,?)`, session, actor, tokenHash[:], now.Add(-time.Minute), now, now.Add(time.Hour), csrfHash[:]).Error; err != nil {
		t.Fatal(err)
	}
	for _, a := range f.request.Command.Authorities {
		var control writefence.Control
		if a.Issuer == writefence.Identity {
			a.Epoch = 0
			control = completedAuthority(t, pgfence.NewAuthorityStore(f.identity), a, false).Control
		} else {
			var head struct{ CoordinationID uuid.UUID }
			if err := f.admin.Raw(`SELECT coordination_id FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind=? AND authority_id=?`, a.Kind, a.ID).Scan(&head).Error; err != nil {
				t.Fatal(err)
			}
			state, err := pgfence.NewAuthorityStore(f.creation).LoadCoordination(t.Context(), head.CoordinationID)
			if err != nil {
				t.Fatal(err)
			}
			control = state.Control
		}
		for _, db := range []*gorm.DB{f.identity, f.creation, f.execution, f.media} {
			if err := pgfence.New(db).Install(t.Context(), control); err != nil {
				t.Fatal(err)
			}
		}
	}
	tokens, names := map[writefence.Writer]string{}, map[writefence.Writer]string{}
	for _, writer := range writefence.Writers() {
		tokens[writer], names[writer] = strings.Repeat(string(writer)+"-", 10), string(writer)
	}
	auth := identity.NewEmailAuth(identitypg.NewStore(f.identity), sourceProofProtector{}, identity.EmailPolicy{}, func() time.Time { return now })
	address := func(writer writefence.Writer) (string, identityrpc.Target) {
		serverTLS, clientTLS, _ := ownerPeerTLS(t, string(writer))
		server, err := identityrpc.NewMTLSServer(auth, identitypg.NewSessionAdmissionStore(f.identity), tokens, names, func() time.Time { return now }, serverTLS)
		if err != nil {
			t.Fatal(err)
		}
		endpoint := serveSourcePipeline(t, server)
		return endpoint, identityrpc.Target{Address: endpoint, Token: tokens[writer], Transport: clientTLS}
	}
	_, mediaTarget := address(writefence.WriterMedia)
	mediaIdentity, err := identityrpc.NewClient(writefence.WriterMedia, mediaTarget)
	if err != nil {
		t.Fatal(err)
	}
	creatorAddress, creatorTarget := address(writefence.WriterCreation)
	creatorIdentity, err := identitysource.New(creatorAddress, creatorTarget.Token, creatorTarget.Transport)
	if err != nil {
		t.Fatal(err)
	}
	return mediaIdentity, creatorIdentity, identityrpc.LegacyCredential(legacy, csrf)
}

func pinAdmissionRequest(t *testing.T, f *sourcePolicyFixture) mediaapp.ReferencePinRequest {
	t.Helper()
	asset, location := uuid.New(), uuid.New()
	profile, key := "pin-admission-"+uuid.NewString(), "pin-admission/"+asset.String()+".png"
	digest := strings.Repeat("b", 64)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES (?,'cos','https://fixture.invalid','https://fixture.invalid','fixture','pin-admission','virtual',?)`, []any{profile, fmt.Sprintf("%x", sha256.Sum256([]byte(profile)))}},
		{`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES (?,?,'pin-admission',?,'accepted',12,?,'image/png')`, []any{location, profile, key, digest}},
		{`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status,moderation_detail,object_location_id) VALUES (?,?,'image','upload','ready',?,'image/png',12,?,'passed','{"number":9007199254740993}',?)`, []any{asset, f.request.ProjectID, key, digest, location}},
	} {
		if err := f.admin.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	command := f.request.Command
	command.Action = "media.reference_pin.acquire"
	return mediaapp.ReferencePinRequest{Command: command, Identity: mediadomain.ReferencePinIdentity{PinID: uuid.New(), Consumer: "creation", ConsumerCommandID: uuid.New(), ConsumerCommandSHA256: strings.Repeat("d", 64), ConsumerResourceID: uuid.New(), ScopeID: f.request.ScopeID, ProjectID: f.request.ProjectID, AssetID: asset, LocationID: location, ByteSize: 12, SHA256: digest, AssetRevision: 1, GateEpoch: 1}}
}

func TestMediaPinAcquisitionUsesOriginalIdentityAndCurrentCreationPolicy(t *testing.T) {
	f := newSourcePolicyFixture(t)
	mediaIdentity, creatorIdentity, credential := pinAdmissionIdentity(t, f)
	request := pinAdmissionRequest(t, f)
	serverTLS, clientTLS, _ := ownerPeerTLS(t, "media")
	token := strings.Repeat("pin-current-source-fixture", 2)
	server, err := grpcpins.NewServer(workspacesource.NewProofAdmission(creatorIdentity, pgsource.New(f.creation)), token, "media", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	client, err := creationpins.New(creationpins.Target{Address: serveSourcePipeline(t, server), Token: token, Transport: clientTLS})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := mediapg.NewReferencePinStore(f.media, nil, client)
	if _, err := store.Acquire(t.Context(), request); err == nil {
		t.Fatal("service identity and installed fences substituted for original Identity admission")
	}
	assertNoPin := func() {
		t.Helper()
		var count int64
		if err := f.media.Table("media.reference_pin").Where("pin_id=?", request.Identity.PinID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("denied source retained a pin: %d %v", count, err)
		}
	}
	assertNoPin()
	if _, err := mediaIdentity.AdmitSessionCommand(t.Context(), credential, request.Command, writefence.WriterMedia); err != nil {
		t.Fatal("original target-Media Identity admission", err)
	}
	if err := f.admin.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, request.Identity.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Acquire(t.Context(), request); err == nil {
		t.Fatal("Identity admission substituted for current Creation project policy")
	}
	assertNoPin()
	if err := f.admin.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, request.Identity.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	pin, err := store.Acquire(t.Context(), request)
	requirePinState(t, pin, err, "held")
	// The original ACK is independent of fresh permits and remote availability.
	server.Stop()
	replay, err := mediapg.NewReferencePinStore(f.media, nil, nil).Acquire(t.Context(), request)
	if err != nil || replay.SnapshotSHA256() != pin.SnapshotSHA256() {
		t.Fatal("accepted original acquisition required a new permit", err)
	}
	request.Command.ID, request.Identity.PinID = uuid.New(), uuid.New()
	if _, err := mediapg.NewReferencePinStore(f.media, nil, nil).Acquire(t.Context(), request); !errors.Is(err, mediadomain.ErrReferencePinProof) {
		t.Fatal("missing source port admitted a new acquisition", err)
	}
}

func TestMediaPinAcquisitionRejectsRealPersonWithoutConsent(t *testing.T) {
	f := newPinFixture(t)
	if err := f.admin.Exec(`UPDATE media.media_asset SET contains_real_person=true,consent_record_id=NULL WHERE id=?`, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire); err == nil {
		t.Fatal("new pin retained an unconsented real-person source")
	}
	var count int64
	if err := f.media.Table("media.reference_pin").Where("pin_id=?", f.acquire.Identity.PinID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("unconsented source produced a pin", count, err)
	}
}

func TestMediaPinAcquisitionRejectsRecycledCatalogSource(t *testing.T) {
	f := newPinFixture(t)
	library := uuid.New()
	if err := f.admin.Exec(`INSERT INTO media.library(id,kind,org_id,project_id) SELECT ?,'project',org_id,id FROM workspace.project WHERE id=?`, library, f.acquire.Identity.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,category,catalog_state,trashed_at,position) VALUES (?,?,?,'recycled source','material','trashed',statement_timestamp(),0)`, f.acquire.Identity.AssetID, library, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire); err == nil {
		t.Fatal("new pin retained a recycled catalog source")
	}
}

type pinAdmissionCall func(context.Context, mediaapp.ReferencePinRequest) (mediaapp.ReferencePinSourceProof, error)

func (f pinAdmissionCall) AdmitReferencePinSource(ctx context.Context, request mediaapp.ReferencePinRequest) (mediaapp.ReferencePinSourceProof, error) {
	return f(ctx, request)
}

func TestMediaPinAcquisitionFinalGuardRejectsClosureAfterRemotePolicy(t *testing.T) {
	f := newPinFixture(t)
	source := pinAdmissionCall(func(ctx context.Context, request mediaapp.ReferencePinRequest) (mediaapp.ReferencePinSourceProof, error) {
		a := request.Command.Authorities[0]
		a.Epoch++
		if err := pgfence.New(f.media).Install(ctx, writefence.Control{Authority: a, Blocked: true, CoordinationID: uuid.New()}); err != nil {
			return mediaapp.ReferencePinSourceProof{}, err
		}
		return (pinSourceFixture{}).AdmitReferencePinSource(ctx, request)
	})
	if _, err := mediapg.NewReferencePinStore(f.media, nil, source).Acquire(t.Context(), f.acquire); !errors.Is(err, writefence.ErrFenced) {
		t.Fatal("source-policy RPC replaced the final Media transaction fence", err)
	}
	var count int64
	if err := f.media.Table("media.reference_pin").Where("pin_id=?", f.acquire.Identity.PinID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("fenced acquisition retained bytes", count, err)
	}
}
