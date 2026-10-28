package subject_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/postgres"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func subjectPostgres(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_SUBJECT_DB_DSN")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires the isolated subject-alignment PostgreSQL database")
		}
		t.Skip("requires an explicitly selected loopback subject-alignment test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || !strings.HasPrefix(parsed.Path, "/glowloom_subject_alignment_") || strings.Contains(strings.TrimPrefix(parsed.Path, "/"), "/") {
		t.Fatal("requires a task-owned subject-alignment database")
	}
	host := parsed.Hostname()
	address := net.ParseIP(host)
	if host != "localhost" && (address == nil || !address.IsLoopback()) {
		t.Fatal("subject integration tests require loopback PostgreSQL")
	}
	for key := range parsed.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected connection override in subject test DSN")
		}
	}
	open := func(connection string) *gorm.DB {
		client, err := platformdb.Open(t.Context(), connection, noop.NewTracerProvider())
		if err != nil {
			t.Fatal("open isolated subject PostgreSQL", err)
		}
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
		return client.DB
	}
	owner := open(dsn)
	query := parsed.Query()
	query.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = query.Encode()
	runtime := open(parsed.String())
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("subject runtime did not assume glowloom_app", err)
	}
	for _, table := range []string{"subject.subject_version", "subject.media_binding", "subject.command"} {
		var mutable bool
		if err := runtime.Raw(`SELECT has_table_privilege(current_user,?,'UPDATE,DELETE')`, table).Scan(&mutable).Error; err != nil || mutable {
			t.Fatalf("immutable table runtime privileges: %s mutable=%v %v", table, mutable, err)
		}
	}
	return runtime, owner
}

func subjectSession(t *testing.T, owner *gorm.DB) identity.Principal {
	t.Helper()
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	if err := owner.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,?)`, actor.OrgID, "subject-pg-"+actor.OrgID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'producer','active',false)`, actor.ID, actor.OrgID, "subject-pg-"+actor.ID.String(), "主体PG测试", "synthetic-fixture-not-a-credential").Error; err != nil {
		t.Fatal(err)
	}
	return identityfixture.Session(t, owner, actor)
}

// These media rows and facts are fixture-proven SQL inputs. No object bytes or
// provider behavior is accepted by this integration test.
func subjectFixtureMedia(t *testing.T, owner *gorm.DB, actor identity.Principal, kind string) domain.MediaFact {
	t.Helper()
	media := fact(kind)
	media.PersonalOrgID = &actor.OrgID
	media.PersonalActorID = &actor.ID
	media.FileName = "subject-fixture.png"
	media.MIMEType = "image/png"
	if kind == "video" {
		media.FileName = "subject-fixture.mp4"
		media.MIMEType = "video/mp4"
		duration := int64(1000)
		media.DurationMS = &duration
	}
	if kind == "audio" {
		media.FileName = "subject-fixture.mp3"
		media.MIMEType = "audio/mpeg"
		duration := int64(1000)
		media.DurationMS = &duration
	}
	key := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/" + kind + "/subject-fixture/" + media.AssetID.String()
	if err := owner.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status,duration_ms) VALUES(?,?,?,?,'upload','ready',?,?,?,?,?,'passed',?)`, media.AssetID, actor.OrgID, actor.ID, kind, key, media.FileName, media.MIMEType, media.ByteSize, media.SHA256, media.DurationMS).Error; err != nil {
		t.Fatal(err)
	}
	return media
}

type realSubjectScope struct {
	resolver workspace.RequestScopeResolver
}

func (s realSubjectScope) Resolve(ctx context.Context, actor identity.Principal, write bool) (subject.ScopeFacts, error) {
	resolved, err := s.resolver.Resolve(ctx, actor, actor.ScopeID, write)
	return subject.ScopeFacts{ScopeID: resolved.ID, PhysicalScopeID: resolved.PhysicalScopeID}, err
}
func subjectFixtureStore(runtime *gorm.DB, media *memoryMedia) *pg.Store {
	return pg.NewStore(runtime, pg.Factories{
		Scope: func(tx *gorm.DB) subject.ScopeAccess {
			return realSubjectScope{resolver: scopewire.ProvideRequestScopeAccess(tx)}
		},
		Media: func(*gorm.DB) subject.MediaAccess { return media },
	})
}

type rejectSubjectReceipt struct{ store *pg.Store }

func (s rejectSubjectReceipt) Write(ctx context.Context, actor identity.Principal, run func(subject.Transaction) (subject.CommandResult, error)) (subject.CommandResult, error) {
	return s.store.Write(ctx, actor, func(tx subject.Transaction) (subject.CommandResult, error) {
		return run(rejectSubjectReceiptTx{Transaction: tx})
	})
}

type rejectSubjectReceiptTx struct{ subject.Transaction }

func (rejectSubjectReceiptTx) Record(context.Context, uuid.UUID, string, subject.CommandResult) error {
	return subject.ErrUnavailable
}

func TestSubjectPostgresAtomicVersionsReplayAndAuthority(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	other := subjectSession(t, owner)
	primary := subjectFixtureMedia(t, owner, actor, "image")
	supplement := subjectFixtureMedia(t, owner, actor, "image")
	voice := subjectFixtureMedia(t, owner, actor, "audio")
	media := &memoryMedia{facts: map[uuid.UUID]domain.MediaFact{primary.AssetID: primary, supplement.AssetID: supplement, voice.AssetID: voice}}
	store := subjectFixtureStore(runtime, media)
	clock := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	service := subject.NewService(store, func() time.Time { return clock })
	input := subject.SaveInput{Name: "PG主体", Description: "fixture-proven media", Primary: subject.MediaSelection{AssetID: primary.AssetID}, References: []subject.MediaSelection{{AssetID: supplement.AssetID}}, Voice: &subject.VoiceSelection{Name: "样本音色", AssetID: voice.AssetID}}
	failedKey := uuid.New()
	if _, err := subject.NewService(rejectSubjectReceipt{store: store}, time.Now).Save(t.Context(), actor, uuid.Nil, failedKey, input); !errors.Is(err, subject.ErrUnavailable) {
		t.Fatal("expected receipt rejection", err)
	}
	for _, table := range []string{"subject.subject_head", "subject.subject_version", "subject.media_binding"} {
		var count int64
		if err := runtime.Raw(`SELECT count(*) FROM `+table+` WHERE owner_scope_id=?`, actor.ScopeID).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("atomic rollback %s: %d %v", table, count, err)
		}
	}
	recovery, err := store.Recover(t.Context(), actor, failedKey)
	if err != nil || recovery.State != "unrecorded" {
		t.Fatal("failed write retained receipt", recovery, err)
	}
	key := uuid.New()
	saved, err := service.Save(t.Context(), actor, uuid.Nil, key, input)
	if err != nil {
		t.Fatal("save", err)
	}
	if saved.Revision != 1 || len(saved.References) != 1 || saved.Voice == nil || saved.Primary.BindingID == uuid.Nil {
		t.Fatal("incomplete saved settings", saved)
	}
	public, err := subjectwire.NewStore(runtime, subjectwire.Objects{}).Get(t.Context(), actor, saved.ID)
	if err != nil || !equalSubjectDetail(public, saved) {
		savedJSON, _ := json.Marshal(saved)
		publicJSON, _ := json.Marshal(public)
		t.Fatalf("real app readback: %v saved=%s read=%s", err, savedJSON, publicJSON)
	}
	bound, err := store.BoundMedia(t.Context(), actor, saved.Primary.BindingID)
	if err != nil || bound.VersionID != saved.VersionID || bound.Media.AssetID != primary.AssetID || bound.BindingSHA256 == "" {
		t.Fatal("immutable binding readback", bound, err)
	}
	version, known, err := store.ReadVersionIfKnown(t.Context(), actor, saved.ID, saved.VersionID)
	if err != nil || !known || version.Content.Name != input.Name {
		t.Fatal("known current version", known, err)
	}
	_, known, err = store.ReadVersionIfKnown(t.Context(), actor, uuid.New(), uuid.New())
	if err != nil || known {
		t.Fatal("unknown legacy candidate", known, err)
	}
	_, known, err = store.ReadVersionIfKnown(t.Context(), other, saved.ID, saved.VersionID)
	if !known || !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("foreign subject allowed legacy fallback", known, err)
	}
	if _, err := store.Get(t.Context(), other, saved.ID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("cross-space read", err)
	}
	if _, err := store.BoundMedia(t.Context(), other, saved.Primary.BindingID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("cross-space binding", err)
	}
	mismatched := actor
	mismatched.OrgID = other.OrgID
	if _, err := store.Get(t.Context(), mismatched, saved.ID); err == nil {
		t.Fatal("logical scope accepted a different physical organization")
	}
	media.fail = true
	replay, err := subject.NewService(subjectFixtureStore(runtime, media), time.Now).Save(t.Context(), actor, uuid.Nil, key, input)
	if err != nil || !equalSubjectDetail(replay, saved) {
		t.Fatal("permanent replay revalidated changed media", err)
	}
	media.fail = false
	changed := input
	changed.Name = "changed"
	if _, err := service.Save(t.Context(), actor, uuid.Nil, key, changed); !errors.Is(err, subject.ErrKeyConflict) {
		t.Fatal("same key accepted changed body", err)
	}
	clock = clock.Add(5 * time.Minute)
	update := input
	update.ExpectedRevision = 1
	update.Name = "PG主体更新"
	newer, err := service.Save(t.Context(), actor, saved.ID, uuid.New(), update)
	if err != nil || newer.Revision != 2 || newer.VersionID == saved.VersionID {
		t.Fatal("versioned update", err)
	}
	if _, err := service.Save(t.Context(), actor, saved.ID, uuid.New(), update); !errors.Is(err, subject.ErrConflict) {
		t.Fatal("stale CAS", err)
	}
	old, err := store.ReadVersion(t.Context(), actor, saved.ID, saved.VersionID)
	if err != nil || old.Content.Name != input.Name {
		t.Fatal("old content changed", err)
	}
	if _, err := store.BoundMedia(t.Context(), actor, saved.Primary.BindingID); err != nil {
		t.Fatal("old binding became unavailable after edit", err)
	}
	if _, err := service.Favorite(t.Context(), actor, saved.ID, uuid.New(), subject.FavoriteInput{ExpectedRevision: 2, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	detail, err := store.Get(t.Context(), actor, saved.ID)
	if err != nil || !detail.Favorite || detail.Revision != 2 || detail.VersionID != newer.VersionID {
		t.Fatal("favorite changed shared content", err)
	}
	var versions int64
	if err := runtime.Raw(`SELECT count(*) FROM subject.subject_version WHERE subject_id=?`, saved.ID).Scan(&versions).Error; err != nil || versions != 2 {
		t.Fatal("favorite rewrote immutable version", versions, err)
	}
	batch, err := service.Batch(t.Context(), actor, uuid.New(), subject.BatchInput{Action: "delete", Items: []subject.BatchItem{{ID: saved.ID, ExpectedRevision: 1}, {ID: uuid.New(), ExpectedRevision: 1}, {ID: saved.ID, ExpectedRevision: 2}}})
	if err == nil || len(batch.Items) != 0 {
		t.Fatal("duplicate batch identity accepted")
	}
	batchKey := uuid.New()
	batchInput := subject.BatchInput{Action: "delete", Items: []subject.BatchItem{{ID: saved.ID, ExpectedRevision: 1}, {ID: uuid.New(), ExpectedRevision: 1}}}
	batch, err = service.Batch(t.Context(), actor, batchKey, batchInput)
	if err != nil || len(batch.Items) != 2 || batch.Items[0].Code != "revision_conflict" || batch.Items[1].Code != "not_found" {
		t.Fatal("batch partial failures", batch, err)
	}
	deleted, err := service.Delete(t.Context(), actor, saved.ID, uuid.New(), subject.RevisionInput{ExpectedRevision: 2})
	if err != nil || deleted.Revision != 3 {
		t.Fatal("delete", err)
	}
	_, known, err = store.ReadVersionIfKnown(t.Context(), actor, saved.ID, saved.VersionID)
	if !known || !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("deleted subject fell back to legacy", known, err)
	}
	if _, err := store.BoundMedia(t.Context(), actor, saved.Primary.BindingID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("deleted subject remained usable", err)
	}
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		used, err := pg.NewMediaGuard(tx).HasRetainedMediaReference(t.Context(), primary.AssetID)
		if err != nil {
			return err
		}
		if !used {
			t.Fatal("deleted subject lost retained-media pin")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity.user_session SET revoked_at=clock_timestamp() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(t.Context(), actor, uuid.Nil, key, input); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("receipt replay bypassed revoked session", err)
	}
	t.Log("real PostgreSQL/role/session/transaction verification; media facts are fixture-only, no object/provider acceptance")
}

func TestSubjectPostgresQueryPaginationSortDatesAndTeamFavorites(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	other := subjectSession(t, owner)
	primary := subjectFixtureMedia(t, owner, actor, "video")
	media := &memoryMedia{facts: map[uuid.UUID]domain.MediaFact{primary.AssetID: primary}}
	store := subjectFixtureStore(runtime, media)
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	clock := base
	service := subject.NewService(store, func() time.Time { return clock })
	all := []subject.Detail{}
	for index, name := range []string{"检索甲", "检索乙", "检索丙", "检索丁"} {
		clock = base.Add(time.Duration(index) * time.Minute)
		if index == 3 {
			clock = base.Add(2 * time.Minute)
		}
		created, err := service.Save(t.Context(), actor, uuid.Nil, uuid.New(), subject.SaveInput{Name: name, Primary: subject.MediaSelection{AssetID: primary.AssetID}})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, created)
	}
	clock = base.Add(5 * time.Minute)
	updated, err := service.Save(t.Context(), actor, all[0].ID, uuid.New(), subject.SaveInput{ExpectedRevision: 1, Name: "检索甲更新", Primary: subject.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	all[0] = updated
	for _, order := range []string{"created_asc", "created_desc", "updated_asc", "updated_desc"} {
		expected := append([]subject.Detail{}, all...)
		sort.Slice(expected, func(i, j int) bool {
			a, b := expected[i].CreatedAt, expected[j].CreatedAt
			if strings.HasPrefix(order, "updated") {
				a, b = expected[i].UpdatedAt, expected[j].UpdatedAt
			}
			comparison := a.Compare(b)
			if comparison == 0 {
				comparison = strings.Compare(expected[i].ID.String(), expected[j].ID.String())
			}
			if strings.HasSuffix(order, "desc") {
				return comparison > 0
			}
			return comparison < 0
		})
		var got []uuid.UUID
		for page := 1; page <= 2; page++ {
			result, err := store.List(t.Context(), actor, subject.ListInput{Query: "检索", TimeField: "updated", Order: order, Page: page, PageSize: 2})
			if err != nil || result.Total != 4 || len(result.Items) != 2 {
				t.Fatalf("%s page %d: %#v %v", order, page, result, err)
			}
			for _, item := range result.Items {
				got = append(got, item.ID)
			}
		}
		want := []uuid.UUID{}
		for _, item := range expected {
			want = append(want, item.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s unstable paged order: got %v want %v", order, got, want)
		}
	}
	from, to := base.Add(time.Minute), base.Add(2*time.Minute)
	page, err := store.List(t.Context(), actor, subject.ListInput{TimeField: "created", From: &from, To: &to, Order: "created_asc", Page: 1, PageSize: 120})
	if err != nil || page.Total != 1 || page.Items[0].ID != all[1].ID {
		t.Fatal("inclusive/exclusive creation-date filter", page, err)
	}
	from, to = base.Add(5*time.Minute), base.Add(6*time.Minute)
	page, err = store.List(t.Context(), actor, subject.ListInput{TimeField: "updated", From: &from, To: &to, Order: "updated_desc", Page: 1, PageSize: 120})
	if err != nil || page.Total != 1 || page.Items[0].ID != all[0].ID {
		t.Fatal("modification date filter", page, err)
	}
	if _, err := service.Favorite(t.Context(), actor, all[2].ID, uuid.New(), subject.FavoriteInput{ExpectedRevision: 1, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	page, err = store.List(t.Context(), actor, subject.ListInput{Query: "检索", Favorite: true, TimeField: "updated", Order: "updated_desc", Page: 1, PageSize: 120})
	if err != nil || page.Total != 1 || page.Items[0].ID != all[2].ID {
		t.Fatal("full-library favorite filter", page, err)
	}
	result, err := service.Batch(t.Context(), actor, uuid.New(), subject.BatchInput{Action: "delete", Items: []subject.BatchItem{{ID: all[0].ID, ExpectedRevision: 2}, {ID: all[1].ID, ExpectedRevision: 99}, {ID: uuid.New(), ExpectedRevision: 1}}})
	if err != nil || result.Items[0].Status != "succeeded" || result.Items[1].Code != "revision_conflict" || result.Items[2].Code != "not_found" {
		t.Fatal("mixed successful batch", result, err)
	}
	teamID, teamScope := uuid.New(), uuid.New()
	if err := owner.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'主体测试团队',?)`, teamID, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range []uuid.UUID{actor.ID, other.ID} {
		if err := owner.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',clock_timestamp(),'invited')`, teamID, member).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, teamScope, teamID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	teamActor, teamOther := actor, other
	teamActor.ScopeID, teamOther.ScopeID = teamScope, teamScope
	teamOther.OrgID = actor.OrgID
	shared, err := service.Save(t.Context(), teamActor, uuid.Nil, uuid.New(), subject.SaveInput{Name: "团队主体", Primary: subject.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal("team subject", err)
	}
	if _, err := service.Favorite(t.Context(), teamActor, shared.ID, uuid.New(), subject.FavoriteInput{ExpectedRevision: 1, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	detail, err := subjectwire.NewStore(runtime, subjectwire.Objects{}).Get(t.Context(), teamOther, shared.ID)
	if err != nil || detail.Favorite {
		t.Fatal("favorite leaked to another authorized team member", err)
	}
	if _, err := store.Get(t.Context(), actor, shared.ID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("same physical organization bypassed logical space", err)
	}
}

// JSON compares the public wire contract, not the location pointer inside time.Time.
func equalSubjectDetail(a, b subject.Detail) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
