package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type avatarMemoryObjects struct {
	data     []byte
	info     media.ObjectInfo
	afterPut func()
	statErr  error
	removed  bool
	ref      mediadomain.ObjectRef
}

func (o *avatarMemoryObjects) PutIfAbsent(_ context.Context, ref mediadomain.ObjectRef, r io.Reader, size int64, mime, hash string) error {
	if ref.Validate() != nil || ref.ProfileID != "identity-fixture" || ref.Bucket != "identity-fixture" {
		return errors.New("unexpected avatar physical target")
	}
	o.ref = ref
	b, err := io.ReadAll(r)
	o.data = b
	o.info = media.ObjectInfo{Size: size, ContentType: mime, SHA256: hash}
	if o.afterPut != nil {
		o.afterPut()
	}
	return err
}
func (o *avatarMemoryObjects) Stat(_ context.Context, ref mediadomain.ObjectRef) (media.ObjectInfo, error) {
	if ref != o.ref {
		return media.ObjectInfo{}, errors.New("avatar stat changed physical target")
	}
	return o.info, o.statErr
}
func (o *avatarMemoryObjects) Remove(context.Context, mediadomain.ObjectRef) error {
	o.removed = true
	return nil
}
func TestAvatarProfileBinding(t *testing.T) {
	tx := existingIdentityTransaction(t)
	seedIdentityStorageRoute(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, a := verifiedTestActor(t, tx, "avatar-owner@example.invalid", now)
	_, b := verifiedTestActor(t, tx, "avatar-other@example.invalid", now)
	var owner struct{ OrgID uuid.UUID }
	if err := tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, a.ID).Scan(&owner).Error; err != nil {
		t.Fatal(err)
	}
	a.OrgID = owner.OrgID
	repo := pgmedia.NewAvatarStore(tx, func(ownerTx *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(ownerTx) })
	objects := &avatarMemoryObjects{}
	s := media.NewRoutedAvatarService(repo, objects, func() time.Time { return now })
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	out, err := s.Upload(t.Context(), a, input.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	profiles := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, func(ownerTx *gorm.DB) identity.AvatarAccess { return pgmedia.NewAvatarAccess(ownerTx) }), func() time.Time { return now })
	before, err := profiles.Read(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := profiles.Update(t.Context(), a, identity.ProfileInput{ExpectedRevision: before.Revision, AvatarID: &out.ID})
	if err != nil || saved.AvatarID == nil || *saved.AvatarID != out.ID {
		t.Fatal("owned normalized avatar unavailable", err)
	}
	other, _ := profiles.Read(t.Context(), b)
	if _, err := profiles.Update(t.Context(), b, identity.ProfileInput{ExpectedRevision: other.Revision, AvatarID: &out.ID}); !errors.Is(err, identity.ErrAvatarUnavailable) {
		t.Fatal("foreign avatar accepted", err)
	}
	if err := tx.Exec(`UPDATE media.media_asset SET moderation_detail='{}' WHERE id=?`, out.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Update(t.Context(), a, identity.ProfileInput{ExpectedRevision: saved.Revision, AvatarID: &out.ID}); !errors.Is(err, identity.ErrAvatarUnavailable) {
		t.Fatal("ordinary image eligible", err)
	}
	cleared, err := profiles.Update(t.Context(), a, identity.ProfileInput{ExpectedRevision: saved.Revision, ClearAvatar: true})
	if err != nil || cleared.AvatarID != nil {
		t.Fatal("clear avatar failed", err)
	}
	objects.afterPut = func() {
		if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, a.SessionID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Upload(t.Context(), a, input.Bytes(), "image/png"); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revocation raced publication", err)
	}
	assertAvatarCancelledWriteRetained(t, tx, repo, a, objects)
	var count int64
	tx.Raw(`SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?`, a.ID).Scan(&count)
	if count != 1 {
		t.Fatal("rejected upload published", count)
	}
}
func TestAvatarSessionAndStorageFailures(t *testing.T) {
	tx := existingIdentityTransaction(t)
	seedIdentityStorageRoute(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, a := verifiedTestActor(t, tx, "avatar-fail@example.invalid", now)
	var owner struct{ OrgID uuid.UUID }
	if err := tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, a.ID).Scan(&owner).Error; err != nil {
		t.Fatal(err)
	}
	a.OrgID = owner.OrgID
	repo := pgmedia.NewAvatarStore(tx, func(ownerTx *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(ownerTx) })
	obj := &avatarMemoryObjects{statErr: errors.New("stored object inaccessible")}
	s := media.NewRoutedAvatarService(repo, obj, func() time.Time { return now })
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload(t.Context(), a, input.Bytes(), "image/png"); err == nil {
		t.Fatal("unverified storage published")
	}
	var count int64
	tx.Raw(`SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?`, a.ID).Scan(&count)
	assertAvatarCancelledWriteRetained(t, tx, repo, a, obj)
	if count != 0 {
		t.Fatal("failed storage registered")
	}
	a.SessionID = uuid.New()
	if err := s.Authorize(t.Context(), a); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("invented session authorized", err)
	}
}

type avatarTestSigner struct {
	afterSign func()
	calls     int
	now       time.Time
}

func (s *avatarTestSigner) PresignGet(_ context.Context, ref mediadomain.ObjectRef, ttl time.Duration) (media.AccessGrant, error) {
	if ref.Validate() != nil || ref.ProfileID != "identity-fixture" || ref.Bucket != "identity-fixture" {
		return media.AccessGrant{}, errors.New("unexpected bound avatar physical identity")
	}
	s.calls++
	if ttl != time.Minute {
		return media.AccessGrant{}, errors.New("unexpected preview window")
	}
	if s.afterSign != nil {
		s.afterSign()
	}
	return media.AccessGrant{URL: "https://private.example.invalid/avatar?signature=test", ExpiresAt: s.now.Add(ttl)}, nil
}
func TestAvatarPreviewRechecksAuthority(t *testing.T) {
	tx := existingIdentityTransaction(t)
	seedIdentityStorageRoute(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, a := verifiedTestActor(t, tx, "avatar-preview@example.invalid", now)
	var owner struct{ OrgID uuid.UUID }
	if err := tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, a.ID).Scan(&owner).Error; err != nil {
		t.Fatal(err)
	}
	a.OrgID = owner.OrgID
	repo := pgmedia.NewAvatarStore(tx, func(db *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(db) })
	objects := &avatarMemoryObjects{}
	s := media.NewRoutedAvatarService(repo, objects, func() time.Time { return now })
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	out, err := s.Upload(t.Context(), a, input.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	var rev int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, a.ID).Scan(&rev)
	profiles := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, func(db *gorm.DB) identity.AvatarAccess { return pgmedia.NewAvatarAccess(db) }), func() time.Time { return now })
	if _, err := profiles.Update(t.Context(), a, identity.ProfileInput{ExpectedRevision: rev, AvatarID: &out.ID}); err != nil {
		t.Fatal(err)
	}
	signer := &avatarTestSigner{now: now}
	q := media.NewRoutedAvatarQuery(repo, signer, func() time.Time { return now }, time.Minute)
	preview, err := q.Read(t.Context(), a)
	if err != nil || preview.Asset.ID != out.ID || preview.URL == "" || !preview.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatal("bound avatar preview failed", err)
	}
	signer.afterSign = func() {
		if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, a.SessionID).Error; err != nil {
			t.Fatal(err)
		}
	}
	preview, err = q.Read(t.Context(), a)
	if !errors.Is(err, identity.ErrSessionInvalid) || preview.URL != "" {
		t.Fatal("revocation during signing exposed preview", err)
	}
	calls := signer.calls
	if _, err := q.Read(t.Context(), a); !errors.Is(err, identity.ErrSessionInvalid) || signer.calls != calls {
		t.Fatal("revoked actor minted another preview", err)
	}
}

func TestAvatarHTTPReadsBoundAvatar(t *testing.T) {
	tx := existingIdentityTransaction(t)
	seedIdentityStorageRoute(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, a := verifiedTestActor(t, tx, "avatar-http@example.invalid", now)
	var owner struct{ OrgID uuid.UUID }
	if err := tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, a.ID).Scan(&owner).Error; err != nil {
		t.Fatal(err)
	}
	a.OrgID = owner.OrgID
	repo := pgmedia.NewAvatarStore(tx, func(db *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(db) })
	s := media.NewRoutedAvatarService(repo, &avatarMemoryObjects{}, func() time.Time { return now })
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		t.Fatal(err)
	}
	out, err := s.Upload(t.Context(), a, imageBytes.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	var rev int64
	tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, a.ID).Scan(&rev)
	profiles := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, func(db *gorm.DB) identity.AvatarAccess { return pgmedia.NewAvatarAccess(db) }), func() time.Time { return now })
	if _, err := profiles.Update(t.Context(), a, identity.ProfileInput{ExpectedRevision: rev, AvatarID: &out.ID}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	signer := &avatarTestSigner{now: now}
	q := media.NewRoutedAvatarQuery(repo, signer, func() time.Time { return now }, time.Minute)
	mediahttp.NewAvatarHandler(s, q).Register(r.Group("/api", func(c *gin.Context) { c.Set("principal", a) }))
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/me/avatar", nil))
		return w
	}
	w := call()
	if w.Code != 200 {
		t.Fatalf("bound avatar HTTP unavailable: %d %s", w.Code, w.Body.String())
	}
	var view media.AvatarPreview
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Asset.ID != out.ID || view.URL == "" || !view.ExpiresAt.Equal(now.Add(time.Minute)) || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || strings.Contains(w.Body.String(), "object_key") {
		t.Fatal("invalid HTTP avatar projection", w.Body.String())
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, a.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if w := call(); w.Code != 401 {
		t.Fatal("revoked session reads avatar", w.Code)
	}
}

// An uncertain write retains its cancelled ownership until every admitted
// provider write has drained. Exercise the real ledger before fake object I/O.
func assertAvatarCancelledWriteRetained(t *testing.T, tx *gorm.DB, repo *pgmedia.AvatarStore, actor identity.Principal, objects *avatarMemoryObjects) {
	t.Helper()
	if objects.removed {
		t.Fatal("in-flight avatar bytes removed before write fence drained")
	}
	var row struct {
		ID, WorkID                          uuid.UUID
		ProfileID, Bucket, ObjectKey, State string
		WriteUntil                          *time.Time
		ClosedAt                            *time.Time
	}
	result := tx.Raw(`SELECT l.id,w.work_id,l.profile_id,l.bucket,l.object_key,w.state,l.write_until,w.closed_at FROM media.object_location l JOIN media.storage_work_intent w ON w.id=l.owner_intent_id WHERE w.kind='avatar' AND w.owner_id=? AND w.state='cancelled'`, actor.ID).Scan(&row)
	if result.Error != nil || result.RowsAffected != 1 || row.WriteUntil == nil || row.ClosedAt == nil || !row.WriteUntil.After(time.Now()) {
		t.Fatal("failed avatar lost durable cancelled write fence", result.Error)
	}
	location := mediadomain.ObjectLocation{ID: row.ID, Ref: mediadomain.ObjectRef{ProfileID: row.ProfileID, Bucket: row.Bucket, Key: row.ObjectKey}}
	if allowed, err := repo.CancelAvatarObject(t.Context(), actor, row.WorkID, location); err != nil || allowed {
		t.Fatal("write fence granted premature removal", err)
	}
}
