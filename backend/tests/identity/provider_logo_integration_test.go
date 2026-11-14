package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type providerLogoFixture struct {
	tx      *gorm.DB
	now     time.Time
	owner   identity.Principal
	other   identity.Principal
	repo    *pgmedia.AvatarStore
	objects *avatarMemoryObjects
	service *media.AvatarService
	image   []byte
}

func newProviderLogoFixture(t *testing.T) providerLogoFixture {
	t.Helper()
	tx := existingIdentityTransaction(t)
	// Other integration suites can have a persistent fixture with the legacy
	// helper's resource fingerprint. Keep this profile isolated and rollback-only.
	resourceIdentity := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	require.NoError(t, tx.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity) VALUES('identity-fixture','cos','http://127.0.0.1:57924','http://127.0.0.1:57924','us-east-1','identity-fixture','',?) ON CONFLICT(id) DO NOTHING`, resourceIdentity).Error)
	seedIdentityStorageRoute(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, owner := verifiedTestActor(t, tx, "logo-owner@example.invalid", now)
	_, other := verifiedTestActor(t, tx, "logo-other@example.invalid", now)
	require.NoError(t, tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1 WHERE id=?`, owner.ID).Error)
	for _, actor := range []*identity.Principal{&owner, &other} {
		var row struct {
			OrgID         uuid.UUID
			PlatformRole  string
			PlatformEpoch int64
		}
		require.NoError(t, tx.Raw(`SELECT org_id,platform_role,platform_epoch FROM identity."user" WHERE id=?`, actor.ID).Scan(&row).Error)
		actor.OrgID, actor.PlatformRole, actor.PlatformEpoch = row.OrgID, row.PlatformRole, row.PlatformEpoch
	}
	repo := pgmedia.NewProviderLogoStore(tx, func(db *gorm.DB) media.AvatarActorAccess { return pgidentity.NewStore(db) })
	objects := &avatarMemoryObjects{}
	service := media.NewRoutedProviderLogoService(repo, objects, func() time.Time { return now })
	var input bytes.Buffer
	require.NoError(t, png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 3, 4))))
	return providerLogoFixture{tx: tx, now: now, owner: owner, other: other, repo: repo, objects: objects, service: service, image: input.Bytes()}
}

func (f providerLogoFixture) upload(t *testing.T) media.LibraryAssetSummary {
	t.Helper()
	asset, err := f.service.Upload(t.Context(), f.owner, f.image, "image/png")
	require.NoError(t, err)
	return asset
}

func (f providerLogoFixture) bind(t *testing.T, assetID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, f.tx.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status,logo_asset_id) VALUES(?,?,'Logo integration','openai','overseas','disabled',?)`, id, "logo-"+id.String(), assetID).Error)
	return id
}

type providerLogoSigner struct {
	avatarTestSigner
	ref mediadomain.ObjectRef
}

func (s *providerLogoSigner) PresignGet(ctx context.Context, ref mediadomain.ObjectRef, ttl time.Duration) (media.AccessGrant, error) {
	s.ref = ref
	return s.avatarTestSigner.PresignGet(ctx, ref, ttl)
}

func TestProviderLogoNormalizedUploadAndCurrentBinding(t *testing.T) {
	f := newProviderLogoFixture(t)
	_, err := f.service.Upload(t.Context(), f.other, f.image, "image/png")
	require.ErrorIs(t, err, identity.ErrForbidden)
	require.Empty(t, f.objects.data, "non-admin must not put an object")
	asset := f.upload(t)
	_, err = png.Decode(bytes.NewReader(f.objects.data))
	require.NoError(t, err, "output must be an actual PNG")
	var detail []byte
	require.NoError(t, f.tx.Raw(`SELECT moderation_detail FROM media.media_asset WHERE id=?`, asset.ID).Row().Scan(&detail))
	var marker map[string]map[string]any
	require.NoError(t, json.Unmarshal(detail, &marker))
	require.Equal(t, "server_reencode_png", marker["provider_logo"]["method"])
	require.Equal(t, float64(1), marker["provider_logo"]["version"])
	require.NotContains(t, marker, "avatar")
	require.ErrorIs(t, pgmedia.NewAvatarAccess(f.tx).Validate(t.Context(), f.owner.ID, asset.ID), identity.ErrAvatarUnavailable)
	var events int64
	require.NoError(t, f.tx.Raw(`SELECT count(*) FROM identity.security_event WHERE actor_id=? AND action='provider_logo.uploaded'`, f.owner.ID).Scan(&events).Error)
	require.Equal(t, int64(1), events)

	signer := &providerLogoSigner{avatarTestSigner: avatarTestSigner{now: f.now}}
	query := media.NewProviderLogoQuery(f.repo, signer, func() time.Time { return f.now }, time.Minute)
	preview, err := query.Read(t.Context(), f.owner, asset.ID)
	require.NoError(t, err)
	require.Equal(t, asset.ID, preview.Asset.ID)
	require.NotEmpty(t, preview.URL)
	require.Equal(t, f.objects.ref, signer.ref)
	_, err = query.Read(t.Context(), f.other, asset.ID)
	require.ErrorIs(t, err, media.ErrNotFound)
	require.Equal(t, 1, signer.calls, "unbound foreign image must not be signed")
	references := pgcatalog.NewProviderLogoReferences(f.tx)
	retained, err := references.HasRetainedMediaReference(t.Context(), asset.ID)
	require.NoError(t, err)
	require.False(t, retained)
	providerID := f.bind(t, asset.ID)
	preview, err = query.Read(t.Context(), f.other, asset.ID)
	require.NoError(t, err)
	require.Equal(t, asset.ID, preview.Asset.ID)
	require.Equal(t, f.objects.ref, signer.ref)
	retained, err = references.HasRetainedMediaReference(t.Context(), asset.ID)
	require.NoError(t, err)
	require.True(t, retained, "bound logos survive purge even while their supplier is disabled")
	require.NoError(t, f.tx.Exec(`UPDATE catalog.provider SET logo_asset_id=NULL WHERE id=?`, providerID).Error)
	_, err = query.Read(t.Context(), f.other, asset.ID)
	require.ErrorIs(t, err, media.ErrNotFound)
	retained, err = references.HasRetainedMediaReference(t.Context(), asset.ID)
	require.NoError(t, err)
	require.False(t, retained)
}

func TestProviderLogoSigningRechecksBindingAndLiveSession(t *testing.T) {
	for _, race := range []string{"unbind", "session-revoke"} {
		t.Run(race, func(t *testing.T) {
			f := newProviderLogoFixture(t)
			asset := f.upload(t)
			providerID := f.bind(t, asset.ID)
			signer := &avatarTestSigner{now: f.now, afterSign: func() {
				if race == "unbind" {
					require.NoError(t, f.tx.Exec(`UPDATE catalog.provider SET logo_asset_id=NULL WHERE id=?`, providerID).Error)
				} else {
					require.NoError(t, f.tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, f.now, f.other.SessionID).Error)
				}
			}}
			query := media.NewProviderLogoQuery(f.repo, signer, func() time.Time { return f.now }, time.Minute)
			preview, err := query.Read(t.Context(), f.other, asset.ID)
			if race == "unbind" {
				require.ErrorIs(t, err, media.ErrNotFound)
			} else {
				require.ErrorIs(t, err, identity.ErrSessionInvalid)
			}
			require.Empty(t, preview.URL)
			require.Equal(t, 1, signer.calls)
			_, err = query.Read(t.Context(), f.other, asset.ID)
			require.Error(t, err)
			require.Equal(t, 1, signer.calls)
		})
	}
}

func TestProviderLogoUploadRechecksLivePlatformAuthority(t *testing.T) {
	f := newProviderLogoFixture(t)
	f.objects.afterPut = func() {
		require.NoError(t, f.tx.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, f.owner.ID).Error)
	}
	asset, err := f.service.Upload(t.Context(), f.owner, f.image, "image/png")
	require.ErrorIs(t, err, identity.ErrForbidden)
	require.Equal(t, uuid.Nil, asset.ID)
	var published int64
	require.NoError(t, f.tx.Raw(`SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?`, f.owner.ID).Scan(&published).Error)
	require.Zero(t, published)
	assertAvatarCancelledWriteRetained(t, f.tx, f.repo, f.owner, f.objects)
}

func TestProviderLogoHTTPAuthorizationAndImageLimits(t *testing.T) {
	f := newProviderLogoFixture(t)
	actor := f.other
	signer := &avatarTestSigner{now: f.now}
	query := media.NewProviderLogoQuery(f.repo, signer, func() time.Time { return f.now }, time.Minute)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	mediahttp.NewProviderLogoHandler(f.service, query).Register(router.Group("/api", func(c *gin.Context) { c.Set("principal", actor) }))
	post := func(content []byte, mime string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		header := textproto.MIMEHeader{"Content-Disposition": []string{`form-data; name="file"; filename="logo.png"`}, "Content-Type": []string{mime}}
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/admin/provider-logos", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusForbidden, post(f.image, "image/png").Code)
	require.Empty(t, f.objects.data)
	actor = f.owner
	require.Equal(t, http.StatusUnsupportedMediaType, post([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml").Code)
	require.Equal(t, http.StatusRequestEntityTooLarge, post(make([]byte, media.AvatarMaxBytes+1), "image/png").Code)
	require.Empty(t, f.objects.data, "rejected input must not reach object storage")
	response := post(f.image, "image/png")
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	var asset media.LibraryAssetSummary
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &asset))
	get := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/provider-logos/"+asset.ID.String(), nil))
		return response
	}
	require.Equal(t, http.StatusOK, get().Code)
	actor = f.other
	require.Equal(t, http.StatusNotFound, get().Code)
	f.bind(t, asset.ID)
	response = get()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
}
