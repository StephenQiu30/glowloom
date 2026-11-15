package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

type pinVerifier func(context.Context, mediadomain.ReferencePin, bool) (mediadomain.ReferencePinProof, error)

func (v pinVerifier) VerifyAdoption(ctx context.Context, pin mediadomain.ReferencePin) (mediadomain.ReferencePinProof, error) {
	return v(ctx, pin, true)
}
func (v pinVerifier) VerifyRelease(ctx context.Context, pin mediadomain.ReferencePin) (mediadomain.ReferencePinProof, error) {
	return v(ctx, pin, false)
}

// pinSourceFixture isolates the local retention mechanics tested in this file.
// The real Identity/Creation acquisition chain has separate restricted-PG RPC tests.
type pinSourceFixture struct{}

func (pinSourceFixture) AdmitReferencePinSource(_ context.Context, request mediaapp.ReferencePinRequest) (mediaapp.ReferencePinSourceProof, error) {
	proof := mediaapp.ReferencePinSourceProof{PhysicalScopeID: uuid.New()}
	for _, a := range request.Command.Authorities {
		if a.Issuer == writefence.Identity && a.Kind == writefence.Account {
			proof.AccountID = a.ID
		}
		if a.Issuer == writefence.Identity && a.Kind == writefence.Session {
			proof.SessionID = a.ID
		}
	}
	return proof, nil
}

type pinFixture struct {
	admin, media *gorm.DB
	acquire      mediaapp.ReferencePinRequest
}

func newPinFixture(t *testing.T) pinFixture {
	t.Helper()
	f := pinFixture{admin: fenceDatabase(t, ""), media: authorityDatabase(t, "media")}
	org, actor, scope, project, asset, location := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	profile, key := "pin-"+uuid.NewString(), "pins/"+asset.String()+".png"
	sha := strings.Repeat("b", 64)
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES (?, 'pin fixture')`, []any{org}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES (?,?,?,'pin fixture','producer','fixture',false)`, []any{actor, org, actor.String()}},
		{`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES (?,'personal',?,?)`, []any{scope, actor, org}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES (?,?,?,?,'pin fixture','16:9','realistic')`, []any{project, org, actor, scope}},
		{`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES (?,'cos','https://fixture.invalid','https://fixture.invalid','fixture','pin-fixture','virtual',?)`, []any{profile, fmt.Sprintf("%x", sha256.Sum256([]byte(profile)))}},
		{`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES (?,?,'pin-fixture',?,'accepted',12,?,'image/png')`, []any{location, profile, key, sha}},
		{`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status,moderation_detail,object_location_id) VALUES (?,?,'image','upload','ready',?,'image/png',12,?,'passed','{"source":"fixture","z":1,"review_code":2,"number":9007199254740993}',?)`, []any{asset, project, key, sha, location}},
	}
	for _, statement := range statements {
		if err := f.admin.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal("pin fixture:", err)
		}
	}
	authorities := []writefence.Authority{
		{Issuer: writefence.Creation, Kind: writefence.Project, ID: project, Epoch: 1},
		{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 1},
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor, Epoch: 1},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New(), Epoch: 1},
	}
	for _, authority := range authorities {
		if err := pgfence.New(f.media).Install(t.Context(), writefence.Control{Authority: authority, CoordinationID: uuid.New()}); err != nil {
			t.Fatal(err)
		}
	}
	f.acquire = mediaapp.ReferencePinRequest{
		Command:  writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.reference_pin.acquire", Authorities: authorities},
		Identity: mediadomain.ReferencePinIdentity{PinID: uuid.New(), Consumer: "creation", ConsumerCommandID: uuid.New(), ConsumerCommandSHA256: strings.Repeat("d", 64), ConsumerResourceID: uuid.New(), ScopeID: scope, ProjectID: project, AssetID: asset, LocationID: location, ByteSize: 12, SHA256: sha, AssetRevision: 1, GateEpoch: 1},
	}
	return f
}

func (f pinFixture) request(action string) mediaapp.ReferencePinRequest {
	req := f.acquire
	req.Command.ID = uuid.New()
	req.Command.Action = "media.reference_pin." + action
	return req
}
func (f pinFixture) closeRequest() mediaapp.CloseReferenceAcquisitionRequest {
	req := f.request("close")
	return mediaapp.CloseReferenceAcquisitionRequest{Command: req.Command, AssetID: f.acquire.Identity.AssetID, Epoch: 1}
}
func requirePinState(t *testing.T, pin mediadomain.ReferencePin, err error, state string) {
	t.Helper()
	if err != nil || pin.State != state {
		t.Fatalf("pin state=%q want=%q err=%v", pin.State, state, err)
	}
}
func pinProof(pin mediadomain.ReferencePin, outcome string) mediadomain.ReferencePinProof {
	raw, _ := json.Marshal(map[string]any{"pin_id": pin.Identity.PinID, "consumer": pin.Identity.Consumer, "command_id": pin.Identity.ConsumerCommandID, "command_sha256": pin.Identity.ConsumerCommandSHA256, "resource_id": pin.Identity.ConsumerResourceID, "snapshot_sha256": pin.SnapshotSHA256(), "outcome": outcome})
	return mediadomain.ReferencePinProof{Identity: pin.Identity, AcquireCommandID: pin.AcquireCommandID, SnapshotSHA256: pin.SnapshotSHA256(), Outcome: outcome, Receipt: raw}
}

func TestMediaReferencePinsRequireMediaOwnerAndIndependentTransaction(t *testing.T) {
	f := newPinFixture(t)
	for _, owner := range []string{"identity", "creation", "execution"} {
		t.Run(owner, func(t *testing.T) {
			db := authorityDatabase(t, owner)
			_, err := mediapg.NewReferencePinStore(db, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
			if !errors.Is(err, mediadomain.ErrReferencePinOwner) {
				t.Fatalf("wrong owner err=%v", err)
			}
			for _, table := range []string{"media.reference_pin", "media.reference_pin_gate"} {
				if err := db.Exec("SELECT * FROM " + table + " LIMIT 0").Error; err == nil {
					t.Fatal("cross owner SELECT allowed", table)
				}
			}
		})
	}
	tx := f.media.Begin()
	defer func() { _ = tx.Rollback().Error }()
	_, err := mediapg.NewReferencePinStore(tx, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
	if !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("outer transaction err=%v", err)
	}
	// The legacy runtime must not bootstrap or erase the new responsibility ledger.
	for _, role := range []string{"glowloom_app", "glowloom_identity", "glowloom_creation", "glowloom_execution"} {
		var granted bool
		if err := f.admin.Raw(`SELECT has_table_privilege(?, 'media.reference_pin', 'SELECT,INSERT,UPDATE,DELETE') OR has_any_column_privilege(?, 'media.reference_pin', 'UPDATE')`, role, role).Scan(&granted).Error; err != nil || granted {
			t.Fatalf("unexpected %s ACL=%t err=%v", role, granted, err)
		}
	}
}

func TestMediaReferencePinsLostACKLifecycleAndPermanentClose(t *testing.T) {
	f := newPinFixture(t)
	store := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
	held, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, held, err, "held")
	original, _ := json.Marshal(held)
	if strings.Contains(string(original), "object_key") || strings.Contains(string(original), "bucket") || !strings.Contains(string(original), "9007199254740993") {
		t.Fatalf("incorrect immutable facts: %s", original)
	}

	// Prove the consumer callback can use the only available Media connection:
	// holding a local SQL transaction while calling this port would deadlock.
	pool, err := f.media.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	verifier := pinVerifier(func(ctx context.Context, p mediadomain.ReferencePin, adopt bool) (mediadomain.ReferencePinProof, error) {
		if err := f.media.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
			return mediadomain.ReferencePinProof{}, err
		}
		outcome := "removed"
		if adopt {
			outcome = "adopted"
		}
		return pinProof(p, outcome), nil
	})
	store = mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{})
	bind := f.request("bind")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	bound, err := store.Bind(ctx, bind)
	requirePinState(t, bound, err, "bound")
	closeReq := f.closeRequest()
	closed, err := store.CloseAcquisition(t.Context(), closeReq)
	if err != nil || !closed.Closed || closed.Epoch != 2 || closed.CloseCommandID == nil || *closed.CloseCommandID != closeReq.Command.ID {
		t.Fatalf("close=%+v err=%v", closed, err)
	}
	replayClose, err := store.CloseAcquisition(t.Context(), closeReq)
	if err != nil || replayClose.Epoch != closed.Epoch {
		t.Fatalf("close replay=%+v err=%v", replayClose, err)
	}
	release := f.request("release")
	released, err := store.Release(t.Context(), release)
	requirePinState(t, released, err, "released")
	if released.AdoptionReceiptSHA256 == "" || released.ReleaseReceiptSHA256 == "" || released.ReleaseOutcome != "removed" {
		t.Fatal("terminal receipts lost", released)
	}
	restarted := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
	acquiredAgain, err := restarted.Acquire(t.Context(), f.acquire)
	requirePinState(t, acquiredAgain, err, "held")
	recovered, _ := json.Marshal(acquiredAgain)
	if string(recovered) != string(original) {
		t.Fatalf("Acquire original held ACK changed: %s != %s", recovered, original)
	}
	bindAgain, err := restarted.Bind(t.Context(), bind)
	requirePinState(t, bindAgain, err, "bound")
	releaseAgain, err := restarted.Release(t.Context(), release)
	requirePinState(t, releaseAgain, err, "released")
	resolved, err := restarted.Resolve(t.Context(), f.request("resolve"))
	requirePinState(t, resolved, err, "released")
	_, err = store.Bind(t.Context(), f.request("bind"))
	if !errors.Is(err, mediadomain.ErrReferencePinConflict) {
		t.Fatalf("released pin resurrected: %v", err)
	}
	next := f.request("acquire")
	next.Identity.PinID = uuid.New()
	_, err = store.Acquire(t.Context(), next)
	if !errors.Is(err, mediadomain.ErrReferencePinClosed) {
		t.Fatalf("new Acquire crossed close: %v", err)
	}
	changed := f.acquire
	changed.Identity.ConsumerResourceID = uuid.New()
	_, err = restarted.Acquire(t.Context(), changed)
	if !errors.Is(err, mediadomain.ErrReferencePinConflict) {
		t.Fatalf("same command changed immutable binding: %v", err)
	}
	if err := f.media.Exec(`DELETE FROM media.reference_pin WHERE pin_id=?`, held.Identity.PinID).Error; err == nil {
		t.Fatal("runtime deleted retained pin")
	}
	if err := f.media.Exec(`UPDATE media.reference_pin SET consumer='execution' WHERE pin_id=?`, held.Identity.PinID).Error; err == nil {
		t.Fatal("runtime changed consumer identity")
	}
	if err := f.media.Exec(`UPDATE media.reference_pin SET state='bound',release_receipt_sha256='',release_outcome='' WHERE pin_id=?`, held.Identity.PinID).Error; err == nil {
		t.Fatal("direct SQL resurrected released pin")
	}
	if err := f.media.Exec(`UPDATE media.reference_pin_gate SET closed=false,close_command_id=NULL WHERE asset_id=?`, held.Identity.AssetID).Error; err == nil {
		t.Fatal("direct SQL reopened closed gate")
	}
}

func TestMediaReferencePinsDecimalFactsKeepOriginalAcquireProof(t *testing.T) {
	f := newPinFixture(t)
	review := `{"decimal":1.2300,"exponent":1e3,"negative_zero":-0.0,"large_integer":9007199254740993}`
	if err := f.admin.Exec(`UPDATE media.media_asset SET moderation_detail=?::jsonb WHERE id=?`, review, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	store := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
	acquired, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, acquired, err, "held")
	wantSnapshot := acquired.SnapshotSHA256()
	wantReview := `{"decimal":1.23,"exponent":1000,"large_integer":9007199254740993,"negative_zero":0}`
	if string(acquired.ModerationDetail) != wantReview {
		t.Fatalf("Acquire review=%s want=%s", acquired.ModerationDetail, wantReview)
	}
	replayed, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, replayed, err, "held")
	resolved, err := store.Resolve(t.Context(), f.request("resolve"))
	requirePinState(t, resolved, err, "held")
	for name, pin := range map[string]mediadomain.ReferencePin{"Acquire replay": replayed, "Resolve reload": resolved} {
		if got := pin.SnapshotSHA256(); got != wantSnapshot {
			t.Errorf("%s snapshot=%s want original=%s", name, got, wantSnapshot)
		}
		if string(pin.ModerationDetail) != wantReview {
			t.Errorf("%s review=%s want=%s", name, pin.ModerationDetail, wantReview)
		}
	}

	// The consumer adopts the exact proof built from the original Acquire ACK,
	// not a replacement constructed from Media's subsequently reloaded Facts.
	originalProof := pinProof(acquired, "adopted")
	var reloadedSnapshot string
	verifier := pinVerifier(func(_ context.Context, pin mediadomain.ReferencePin, _ bool) (mediadomain.ReferencePinProof, error) {
		reloadedSnapshot = pin.SnapshotSHA256()
		return originalProof, nil
	})
	bound, err := mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{}).Bind(t.Context(), f.request("bind"))
	if reloadedSnapshot != wantSnapshot {
		t.Errorf("persisted Facts snapshot=%s want original Acquire=%s", reloadedSnapshot, wantSnapshot)
	}
	requirePinState(t, bound, err, "bound")
	if got := bound.SnapshotSHA256(); got != wantSnapshot {
		t.Fatalf("Bind snapshot=%s want original=%s", got, wantSnapshot)
	}
}

func TestMediaReferencePinsRejectMismatchedAndUnknownProof(t *testing.T) {
	f := newPinFixture(t)
	held, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
	requirePinState(t, held, err, "held")
	changes := map[string]func(*mediadomain.ReferencePinProof){
		"pin":            func(p *mediadomain.ReferencePinProof) { p.Identity.PinID = uuid.New() },
		"consumer":       func(p *mediadomain.ReferencePinProof) { p.Identity.Consumer = "execution" },
		"command":        func(p *mediadomain.ReferencePinProof) { p.Identity.ConsumerCommandID = uuid.New() },
		"command_digest": func(p *mediadomain.ReferencePinProof) { p.Identity.ConsumerCommandSHA256 = strings.Repeat("e", 64) },
		"resource":       func(p *mediadomain.ReferencePinProof) { p.Identity.ConsumerResourceID = uuid.New() },
		"scope":          func(p *mediadomain.ReferencePinProof) { p.Identity.ScopeID = uuid.New() },
		"project":        func(p *mediadomain.ReferencePinProof) { p.Identity.ProjectID = uuid.New() },
		"asset":          func(p *mediadomain.ReferencePinProof) { p.Identity.AssetID = uuid.New() },
		"location":       func(p *mediadomain.ReferencePinProof) { p.Identity.LocationID = uuid.New() },
		"size":           func(p *mediadomain.ReferencePinProof) { p.Identity.ByteSize++ },
		"sha":            func(p *mediadomain.ReferencePinProof) { p.Identity.SHA256 = strings.Repeat("e", 64) },
		"revision":       func(p *mediadomain.ReferencePinProof) { p.Identity.AssetRevision++ },
		"epoch":          func(p *mediadomain.ReferencePinProof) { p.Identity.GateEpoch++ },
		"acquire":        func(p *mediadomain.ReferencePinProof) { p.AcquireCommandID = uuid.New() },
		"snapshot":       func(p *mediadomain.ReferencePinProof) { p.SnapshotSHA256 = strings.Repeat("e", 64) },
		"404":            func(p *mediadomain.ReferencePinProof) { p.Outcome = "not_found" },
		"ttl":            func(p *mediadomain.ReferencePinProof) { p.Outcome = "expired" },
		"self_report":    func(p *mediadomain.ReferencePinProof) { p.Outcome = "not_adopted" },
		"empty_receipt":  func(p *mediadomain.ReferencePinProof) { p.Receipt = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			verifier := pinVerifier(func(_ context.Context, p mediadomain.ReferencePin, _ bool) (mediadomain.ReferencePinProof, error) {
				proof := pinProof(p, "not_adopted_frozen")
				change(&proof)
				return proof, nil
			})
			_, err := mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{}).Release(t.Context(), f.request("release"))
			if !errors.Is(err, mediadomain.ErrReferencePinProof) {
				t.Fatalf("mismatched proof err=%v", err)
			}
		})
	}
	unavailable := errors.New("consumer deadline exceeded")
	verifier := pinVerifier(func(context.Context, mediadomain.ReferencePin, bool) (mediadomain.ReferencePinProof, error) {
		return mediadomain.ReferencePinProof{}, unavailable
	})
	_, err = mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{}).Release(t.Context(), f.request("release"))
	if !errors.Is(err, unavailable) {
		t.Fatal("unknown consumer evidence accepted", err)
	}
	resolved, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Resolve(t.Context(), f.request("resolve"))
	requirePinState(t, resolved, err, "held")
}

func TestMediaReferencePinsReleaseRequiresOriginalConsumerFrozenCommand(t *testing.T) {
	f := newPinFixture(t)
	consumerDB := authorityDatabase(t, "creation")
	consumerFence := pgfence.New(consumerDB)
	authority := f.acquire.Command.Authorities[0]
	if err := consumerFence.Install(t.Context(), writefence.Control{Authority: authority, CoordinationID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	consumer := writefence.Command{ID: f.acquire.Identity.ConsumerCommandID, SHA256: f.acquire.Identity.ConsumerCommandSHA256, Action: "creation.reference.adopt", Authorities: []writefence.Authority{authority}}
	if _, err := consumerFence.Admit(t.Context(), consumer); err != nil {
		t.Fatal(err)
	}
	verifier := pinVerifier(func(ctx context.Context, p mediadomain.ReferencePin, _ bool) (mediadomain.ReferencePinProof, error) {
		record, err := consumerFence.Load(ctx, p.Identity.ConsumerCommandID)
		if err != nil {
			return mediadomain.ReferencePinProof{}, err
		}
		if record.State != writefence.Frozen || record.Command.SHA256 != p.Identity.ConsumerCommandSHA256 {
			return mediadomain.ReferencePinProof{}, mediadomain.ErrReferencePinProof
		}
		return pinProof(p, "not_adopted_frozen"), nil
	})
	store := mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{})
	held, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, held, err, "held")
	release := f.request("release")
	if _, err := store.Release(t.Context(), release); !errors.Is(err, mediadomain.ErrReferencePinProof) {
		t.Fatalf("still-admitted consumer released: %v", err)
	}
	if _, err := consumerFence.Freeze(t.Context(), consumer.ID); err != nil {
		t.Fatal(err)
	}
	released, err := store.Release(t.Context(), release)
	requirePinState(t, released, err, "released")
	err = consumerDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error { return consumerFence.Guard(t.Context(), tx, consumer) })
	if !errors.Is(err, writefence.ErrFrozen) {
		t.Fatalf("late consumer commit allowed: %v", err)
	}
}

func TestMediaReferencePinsAcquireChecksExactLiveFacts(t *testing.T) {
	changes := map[string]func(*mediaapp.ReferencePinRequest){
		"project":  func(r *mediaapp.ReferencePinRequest) { r.Identity.ProjectID = uuid.New() },
		"location": func(r *mediaapp.ReferencePinRequest) { r.Identity.LocationID = uuid.New() },
		"bytes":    func(r *mediaapp.ReferencePinRequest) { r.Identity.ByteSize++ },
		"sha":      func(r *mediaapp.ReferencePinRequest) { r.Identity.SHA256 = strings.Repeat("e", 64) },
		"revision": func(r *mediaapp.ReferencePinRequest) { r.Identity.AssetRevision++ },
		"epoch":    func(r *mediaapp.ReferencePinRequest) { r.Identity.GateEpoch++ },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			f := newPinFixture(t)
			req := f.acquire
			change(&req)
			if _, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), req); err == nil {
				t.Fatal("Acquire accepted incorrect", name)
			}
			var count int64
			if err := f.admin.Table("media.reference_pin").Where("pin_id=?", req.Identity.PinID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("failed Acquire left pin=%d err=%v", count, err)
			}
		})
	}
	for _, change := range []struct{ name, sql string }{
		{"not_ready", `UPDATE media.media_asset SET status='processing' WHERE id=?`},
		{"review_pending", `UPDATE media.media_asset SET moderation_status='pending' WHERE id=?`},
		{"deleted", `UPDATE media.media_asset SET is_delete=true WHERE id=?`},
		{"location_not_accepted", `UPDATE media.object_location SET state='present' WHERE id=?`},
		{"location_sha", `UPDATE media.object_location SET sha256='` + strings.Repeat("e", 64) + `' WHERE id=?`},
	} {
		t.Run(change.name, func(t *testing.T) {
			f := newPinFixture(t)
			id := f.acquire.Identity.AssetID
			if strings.HasPrefix(change.name, "location_") {
				id = f.acquire.Identity.LocationID
			}
			if err := f.admin.Exec(change.sql, id).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire); err == nil {
				t.Fatal("Acquire accepted", change.name)
			}
		})
	}
}

func TestMediaReferencePinsGuardRollbackAndFrozenCommand(t *testing.T) {
	f := newPinFixture(t)
	fence := pgfence.New(f.media)
	if _, err := fence.Admit(t.Context(), f.acquire.Command); err != nil {
		t.Fatal(err)
	}
	control := writefence.Control{Authority: f.acquire.Command.Authorities[0], CoordinationID: uuid.New(), Blocked: true}
	control.Epoch++
	if err := fence.Install(t.Context(), control); err != nil {
		t.Fatal(err)
	}
	_, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
	if !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("stale command acquired: %v", err)
	}
	var count int64
	if err := f.admin.Table("media.reference_pin_gate").Where("asset_id=?", f.acquire.Identity.AssetID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("Guard failure left gate=%d err=%v", count, err)
	}
	f2 := newPinFixture(t)
	if _, err := pgfence.New(f2.media).Admit(t.Context(), f2.acquire.Command); err != nil {
		t.Fatal(err)
	}
	if _, err := pgfence.New(f2.media).Freeze(t.Context(), f2.acquire.Command.ID); err != nil {
		t.Fatal(err)
	}
	_, err = mediapg.NewReferencePinStore(f2.media, nil, pinSourceFixture{}).Acquire(t.Context(), f2.acquire)
	if !errors.Is(err, writefence.ErrFrozen) {
		t.Fatalf("frozen Acquire committed: %v", err)
	}
	// Failure after a gate INSERT rolls back both the gate and the effect, and
	// leaves the admitted original command available for exact retry.
	f3 := newPinFixture(t)
	if err := f3.admin.Exec(`UPDATE media.object_location SET state='present' WHERE id=?`, f3.acquire.Identity.LocationID).Error; err != nil {
		t.Fatal(err)
	}
	store := mediapg.NewReferencePinStore(f3.media, nil, pinSourceFixture{})
	if _, err := store.Acquire(t.Context(), f3.acquire); err == nil {
		t.Fatal("unaccepted location acquired")
	}
	if err := f3.admin.Table("media.reference_pin_gate").Where("asset_id=?", f3.acquire.Identity.AssetID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("effect failure retained gate=%d err=%v", count, err)
	}
	record, err := pgfence.New(f3.media).Load(t.Context(), f3.acquire.Command.ID)
	if err != nil || record.State != writefence.Admitted {
		t.Fatalf("effect failure changed command=%+v err=%v", record, err)
	}
	if err := f3.admin.Exec(`UPDATE media.object_location SET state='accepted' WHERE id=?`, f3.acquire.Identity.LocationID).Error; err != nil {
		t.Fatal(err)
	}
	pin, err := store.Acquire(t.Context(), f3.acquire)
	requirePinState(t, pin, err, "held")
}

func TestMediaReferencePinsFinalGuardSerializesRevocation(t *testing.T) {
	f := newPinFixture(t)
	if err := f.media.Exec(`INSERT INTO media.reference_pin_gate(asset_id) VALUES (?)`, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	tx := f.admin.WithContext(ctx).Begin()
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec(`SELECT * FROM media.reference_pin_gate WHERE asset_id=? FOR UPDATE`, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	var gateHolder int
	if err := tx.Raw("SELECT pg_backend_pid()").Scan(&gateHolder).Error; err != nil {
		t.Fatal(err)
	}
	store := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
	acquireDone := make(chan error, 1)
	go func() { _, err := store.Acquire(ctx, f.acquire); acquireDone <- err }()
	defer func() { _ = tx.Rollback().Error; cancel() }()
	if err := waitPinBlocked(ctx, f.admin, gateHolder, acquireDone); err != nil {
		t.Fatal(err)
	}
	var acquirer int
	if err := f.admin.WithContext(ctx).Raw(`SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND ?=ANY(pg_blocking_pids(pid)) AND query LIKE '%reference_pin_gate%' LIMIT 1`, gateHolder).Scan(&acquirer).Error; err != nil || acquirer == 0 {
		t.Fatalf("identify final Guard transaction: pid=%d err=%v", acquirer, err)
	}
	control := writefence.Control{Authority: f.acquire.Command.Authorities[0], CoordinationID: uuid.New(), Blocked: true}
	control.Epoch++
	revoked := make(chan error, 1)
	go func() { revoked <- pgfence.New(f.media).Install(ctx, control) }()
	if err := waitPinBlocked(ctx, f.admin, acquirer, revoked); err != nil {
		t.Fatal("revocation escaped final Guard:", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-acquireDone; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	replay, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, replay, err, "held")
	if _, err := store.Resolve(t.Context(), f.request("resolve")); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("new command crossed revocation: %v", err)
	}
}

func TestMediaReferencePinsCompleteFailureRollsBackEffect(t *testing.T) {
	f := newPinFixture(t)
	// A valid but oversized review snapshot reaches the effect INSERT, then the
	// existing writefence receipt limit rejects Complete. No pin/gate may survive.
	if err := f.admin.Exec(`UPDATE media.media_asset SET moderation_detail=jsonb_build_object('evidence',repeat('x',?)) WHERE id=?`, writefence.MaxReceiptBytes, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	_, err := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{}).Acquire(t.Context(), f.acquire)
	if !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("Complete receipt bound err=%v", err)
	}
	for _, table := range []string{"media.reference_pin", "media.reference_pin_gate"} {
		var count int64
		if err := f.admin.Table(table).Where("asset_id=?", f.acquire.Identity.AssetID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("Complete failure retained %s rows=%d err=%v", table, count, err)
		}
	}
	record, err := pgfence.New(f.media).Load(t.Context(), f.acquire.Command.ID)
	if err != nil || record.State != writefence.Admitted {
		t.Fatalf("Complete failure record=%+v err=%v", record, err)
	}
}

func TestMediaReferencePinsVerifiesDurableConsumerAdoptionAndRemoval(t *testing.T) {
	f := newPinFixture(t)
	f.acquire.Identity.Consumer = "execution"
	consumerDB := authorityDatabase(t, "execution")
	fence := pgfence.New(consumerDB)
	for _, authority := range f.acquire.Command.Authorities {
		if err := fence.Install(t.Context(), writefence.Control{Authority: authority, CoordinationID: uuid.New()}); err != nil {
			t.Fatal(err)
		}
	}
	original := writefence.Command{ID: f.acquire.Identity.ConsumerCommandID, SHA256: f.acquire.Identity.ConsumerCommandSHA256, Action: "execution.reference.adopt", Authorities: f.acquire.Command.Authorities}
	removal := original
	removal.ID = uuid.New()
	removal.Action = "execution.reference.remove"
	for _, command := range []writefence.Command{original, removal} {
		if _, err := fence.Admit(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
	verifier := pinVerifier(func(ctx context.Context, p mediadomain.ReferencePin, adopt bool) (mediadomain.ReferencePinProof, error) {
		originalRecord, err := fence.Load(ctx, p.Identity.ConsumerCommandID)
		if err != nil {
			return mediadomain.ReferencePinProof{}, err
		}
		if originalRecord.Command.SHA256 != p.Identity.ConsumerCommandSHA256 || originalRecord.State != writefence.Committed {
			return mediadomain.ReferencePinProof{}, mediadomain.ErrReferencePinProof
		}
		expected := pinProof(p, "adopted")
		canonical, err := writefence.CanonicalReceipt(expected.Receipt)
		if err != nil || string(originalRecord.Receipt) != string(canonical) {
			return mediadomain.ReferencePinProof{}, mediadomain.ErrReferencePinProof
		}
		if adopt {
			return expected, nil
		}
		removed, err := fence.Load(ctx, removal.ID)
		if err != nil {
			return mediadomain.ReferencePinProof{}, err
		}
		expected = pinProof(p, "removed")
		canonical, err = writefence.CanonicalReceipt(expected.Receipt)
		if err != nil || removed.State != writefence.Committed || string(removed.Receipt) != string(canonical) {
			return mediadomain.ReferencePinProof{}, mediadomain.ErrReferencePinProof
		}
		return expected, nil
	})
	store := mediapg.NewReferencePinStore(f.media, verifier, pinSourceFixture{})
	held, err := store.Acquire(t.Context(), f.acquire)
	requirePinState(t, held, err, "held")
	bind := f.request("bind")
	if _, err := store.Bind(t.Context(), bind); !errors.Is(err, mediadomain.ErrReferencePinProof) {
		t.Fatalf("uncommitted adoption accepted: %v", err)
	}
	complete := func(command writefence.Command, outcome string) {
		t.Helper()
		err := consumerDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
			if err := fence.Guard(t.Context(), tx, command); err != nil {
				return err
			}
			return fence.Complete(t.Context(), tx, command, pinProof(held, outcome).Receipt)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	complete(original, "adopted")
	bound, err := store.Bind(t.Context(), bind)
	requirePinState(t, bound, err, "bound")
	release := f.request("release")
	if _, err := store.Release(t.Context(), release); !errors.Is(err, mediadomain.ErrReferencePinProof) {
		t.Fatalf("uncommitted removal accepted: %v", err)
	}
	complete(removal, "removed")
	released, err := store.Release(t.Context(), release)
	requirePinState(t, released, err, "released")
}

// waitPinBlocked observes PostgreSQL's real lock graph; elapsed time alone is
// never evidence that the competing transaction reached its gate.
func waitPinBlocked(ctx context.Context, admin *gorm.DB, holder int, done <-chan error) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := admin.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND ?=ANY(pg_blocking_pids(pid)))`, holder).Scan(&blocked).Error; err != nil {
			return err
		}
		if blocked {
			return nil
		}
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("operation failed before gate blocking: %w", err)
			}
			return errors.New("operation finished before gate blocking")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestMediaReferencePinsAcquireAndCloseUseSameAssetGate(t *testing.T) {
	for _, closeFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("close_first_%t", closeFirst), func(t *testing.T) {
			f := newPinFixture(t)
			// Seed the permanent asset gate offline through its normal Media ACL; the
			// holder then models the exact gate write performed by the opposing action.
			if err := f.media.Exec(`INSERT INTO media.reference_pin_gate(asset_id) VALUES (?)`, f.acquire.Identity.AssetID).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			tx := f.media.WithContext(ctx).Begin()
			defer func() { _ = tx.Rollback().Error }()
			if err := tx.Exec(`SELECT * FROM media.reference_pin_gate WHERE asset_id=? FOR UPDATE`, f.acquire.Identity.AssetID).Error; err != nil {
				t.Fatal(err)
			}
			var holder int
			if err := tx.Raw("SELECT pg_backend_pid()").Scan(&holder).Error; err != nil {
				t.Fatal(err)
			}
			store := mediapg.NewReferencePinStore(f.media, nil, pinSourceFixture{})
			done := make(chan error, 1)
			if closeFirst {
				// Close the gate but do not commit yet. Acquire must wait, then see closed.
				if err := tx.Exec(`UPDATE media.reference_pin_gate SET closed=true,epoch=2,close_command_id=? WHERE asset_id=?`, uuid.New(), f.acquire.Identity.AssetID).Error; err != nil {
					t.Fatal(err)
				}
				go func() { _, err := store.Acquire(ctx, f.acquire); done <- err }()
			} else {
				go func() { _, err := store.CloseAcquisition(ctx, f.closeRequest()); done <- err }()
			}
			defer func() { _ = tx.Rollback().Error; cancel() }()
			if err := waitPinBlocked(ctx, f.admin, holder, done); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
			err := <-done
			if closeFirst && !errors.Is(err, mediadomain.ErrReferencePinClosed) || !closeFirst && err != nil {
				t.Fatalf("competing gate result=%v", err)
			}
		})
	}
}
