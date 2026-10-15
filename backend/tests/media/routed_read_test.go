package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type routedReadObjects struct {
	grants map[domain.ObjectRef]mediaapp.AccessGrant
	data   map[domain.ObjectRef][]byte
	err    error
	calls  int
	onSign func()
}

func (s *routedReadObjects) PresignGet(_ context.Context, ref domain.ObjectRef, _ time.Duration) (mediaapp.AccessGrant, error) {
	s.calls++
	if s.onSign != nil {
		s.onSign()
	}
	if s.err != nil {
		return mediaapp.AccessGrant{}, s.err
	}
	g, ok := s.grants[ref]
	if !ok {
		return mediaapp.AccessGrant{}, errors.New("unexpected physical reference")
	}
	return g, nil
}
func (s *routedReadObjects) Get(_ context.Context, ref domain.ObjectRef) (io.ReadCloser, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	b, ok := s.data[ref]
	if !ok {
		return nil, errors.New("object missing")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func readLocation(profile, key string) *domain.ObjectLocation {
	return &domain.ObjectLocation{ID: uuid.New(), Ref: domain.ObjectRef{ProfileID: profile, Bucket: "storage-test", Key: key}, State: "accepted"}
}
func TestRoutedPreviewUsesSavedProfileAndActualGrantExpiry(t *testing.T) {
	a := routedReadImage()
	a.ObjectKey = "projects/" + a.ProjectID.String() + "/image/same.png"
	a.FileName = "same.png"
	a.MimeType = "image/png"
	codec := "png"
	a.Codec = &codec
	expiry := time.Now().UTC().Add(47 * time.Second).Truncate(time.Second)
	for _, profile := range []string{"minio-main", "cos-main", "tos-main"} {
		a.ObjectLocation = readLocation(profile, a.ObjectKey)
		objects := &routedReadObjects{grants: map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://" + profile + ".invalid/private", ExpiresAt: expiry}}}
		query := mediaapp.NewRoutedAssetQuery(readStore{asset: a}, objects, time.Minute)
		got, err := query.Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID)
		if err != nil || !got.ExpiresAt.Equal(expiry) || !strings.Contains(got.URL, profile) {
			t.Fatal("saved profile or real credential expiry lost", err)
		}
	}
}
func TestRoutedPreviewRejectsMissingLocationAndReauthorizesBeforeSigning(t *testing.T) {
	a := routedReadImage()
	objects := &routedReadObjects{}
	query := mediaapp.NewRoutedAssetQuery(readStore{asset: a}, objects, time.Minute)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, mediaapp.ErrUnavailable) || objects.calls != 0 {
		t.Fatal("unproven legacy location guessed", err)
	}
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	query = mediaapp.NewRoutedAssetQuery(readStore{asset: a, err: identityapp.ErrForbidden}, objects, time.Minute)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, identityapp.ErrForbidden) || objects.calls != 0 {
		t.Fatal("revoked actor got new signature", err)
	}
	for _, expiry := range []time.Time{time.Now().Add(-time.Minute), time.Now().Add(16 * time.Minute)} {
		objects.grants = map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://fixture.invalid/private", ExpiresAt: expiry}}
		query = mediaapp.NewRoutedAssetQuery(readStore{asset: a}, objects, time.Minute)
		if _, err := query.Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, mediaapp.ErrUnavailable) {
			t.Fatal("invalid lease published", err)
		}
	}
	legacy := &previewSigner{}
	if _, err := mediaapp.NewAssetQuery(readStore{asset: a}, legacy).Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID); !errors.Is(err, mediaapp.ErrUnavailable) || legacy.calls != 0 {
		t.Fatal("routed record fell back to original bucket", err)
	}
}

type routedLibraryReader struct{ file mediaapp.LibraryMediaFile }

func (r routedLibraryReader) FindLibraryMedia(context.Context, identityapp.Principal, domain.LibraryScope, uuid.UUID) (mediaapp.LibraryMediaFile, error) {
	return r.file, nil
}
func TestRoutedLibraryDownloadNeverFallsBackToSameKeyInAnotherProfile(t *testing.T) {
	data := []byte("historical original")
	a := sourceDocumentAsset(data)
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	other := a.ObjectLocation.Ref
	other.ProfileID = "cos-main"
	objects := &routedReadObjects{data: map[domain.ObjectRef][]byte{other: []byte("wrong bytes")}}
	repo := routedLibraryReader{file: mediaapp.LibraryMediaFile{Asset: a}}
	query := mediaapp.NewRoutedLibraryMediaQuery(repo, objects, objects, time.Minute)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &a.ProjectID}
	if file, _, err := query.Download(t.Context(), identityapp.Principal{}, scope, a.ID); file != nil || !errors.Is(err, mediaapp.ErrUnavailable) || objects.calls != 1 {
		t.Fatal("missing original searched another profile", err)
	}
	objects.data[a.ObjectLocation.Ref] = data
	file, _, err := query.Download(t.Context(), identityapp.Principal{}, scope, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	got, err := io.ReadAll(file.File)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("historical bytes changed", err)
	}
	objects.err = errors.New("provider 403")
	if _, _, err := query.Download(t.Context(), identityapp.Principal{}, scope, a.ID); !errors.Is(err, objects.err) {
		t.Fatal("authorization failure lost", err)
	}
}

func TestRoutedLibraryRenditionsKeepIndependentProfilesAndExpiries(t *testing.T) {
	a := routedReadImage()
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	rend := domain.Rendition{ID: uuid.New(), MediaAssetID: a.ID, Kind: domain.RenditionThumb256, ObjectKey: strings.TrimSuffix(a.ObjectKey, ".png") + "/thumb_256.png"}
	rend.ObjectLocation = readLocation("cos-main", rend.ObjectKey)
	expiry := time.Now().UTC().Add(45 * time.Second)
	rendExpiry := expiry.Add(-10 * time.Second)
	objects := &routedReadObjects{grants: map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://minio.invalid/original", ExpiresAt: expiry}, rend.ObjectLocation.Ref: {URL: "https://cos.invalid/thumb", ExpiresAt: rendExpiry}}}
	query := mediaapp.NewRoutedLibraryMediaQuery(routedLibraryReader{file: mediaapp.LibraryMediaFile{Asset: a, Renditions: []domain.Rendition{rend}}}, objects, objects, time.Minute)
	result, err := query.Preview(t.Context(), identityapp.Principal{}, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &a.ProjectID}, a.ID)
	if err != nil || len(result.Renditions) != 1 || result.URL != "https://minio.invalid/original" || !result.ExpiresAt.Equal(expiry) || result.Renditions[0].URL != "https://cos.invalid/thumb" || !result.Renditions[0].ExpiresAt.Equal(rendExpiry) {
		t.Fatal("original or rendition routed by default or fabricated expiry", err)
	}
}

func TestRoutedDocumentSourceUsesSavedLocationAfterFreeze(t *testing.T) {
	data := []byte("frozen script original")
	a := sourceDocumentAsset(data)
	a.ObjectLocation = readLocation("tos-main", a.ObjectKey)
	repo := &documentSourceRepo{assets: []domain.MediaAsset{a}}
	objects := &routedReadObjects{data: map[domain.ObjectRef][]byte{a.ObjectLocation.Ref: data}}
	source := mediaapp.NewRoutedDocumentSources(repo, objects)
	frozen, err := source.Freeze(t.Context(), identityapp.Principal{}, a.ProjectID, []uuid.UUID{a.ID})
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Open(t.Context(), identityapp.Principal{}, a.ProjectID, frozen[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	got, err := io.ReadAll(file.File)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("frozen source changed", err)
	}
	legacy := &documentSourceObjects{data: data}
	if _, err := mediaapp.NewDocumentSources(repo, legacy).Open(t.Context(), identityapp.Principal{}, a.ProjectID, frozen[0]); !errors.Is(err, mediaapp.ErrUnavailable) || legacy.calls != 0 {
		t.Fatal("routed document fell back to original bucket", err)
	}
}

func routedReadImage() domain.MediaAsset {
	a := sourceDocumentAsset([]byte("image"))
	a.Kind = domain.KindImage
	a.ObjectKey = strings.Replace(strings.TrimSuffix(a.ObjectKey, ".txt"), "/document/", "/image/", 1) + ".png"
	a.FileName = "original.png"
	a.MimeType = "image/png"
	codec := "png"
	a.Codec = &codec
	return a
}

type routedReferenceReader struct{ file mediaapp.LibraryMediaFile }

func (r routedReferenceReader) ReadReferenceFactSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, domain.Kind) (mediaapp.LibraryMediaFile, error) {
	return r.file, nil
}
func (r routedReferenceReader) ReadFrozenReferenceFactSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, domain.Kind) (mediaapp.LibraryMediaFile, error) {
	return r.file, nil
}
func TestRoutedReferenceVerifiesOriginalAndFrozenRenditionLocations(t *testing.T) {
	data := uploadPNG(t)
	a := routedReadImage()
	facts := sourceDocumentAsset(data)
	a.SHA256 = facts.SHA256
	a.ByteSize = facts.ByteSize
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	rend := domain.Rendition{ID: uuid.New(), MediaAssetID: a.ID, Kind: domain.RenditionThumb256, ObjectKey: strings.TrimSuffix(a.ObjectKey, ".png") + "/thumb_256.png", ByteSize: &a.ByteSize, CreateTime: a.CreateTime, UpdateTime: a.UpdateTime}
	rend.ObjectLocation = readLocation("tos-main", rend.ObjectKey)
	objects := &routedReadObjects{data: map[domain.ObjectRef][]byte{a.ObjectLocation.Ref: data, rend.ObjectLocation.Ref: data}}
	reader := routedReferenceReader{file: mediaapp.LibraryMediaFile{Asset: a, Renditions: []domain.Rendition{rend}}}
	query := mediaapp.NewRoutedReferenceFactQuery(reader, objects)
	fact, err := query.Reference(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID, domain.KindImage)
	if err != nil {
		t.Fatal(err)
	}
	if err := query.VerifyFrozenReference(t.Context(), identityapp.Principal{}, a.ProjectID, fact); err != nil {
		t.Fatal("frozen byte verification lost locations", err)
	}
	delete(objects.data, rend.ObjectLocation.Ref)
	if err := query.VerifyFrozenReference(t.Context(), identityapp.Principal{}, a.ProjectID, fact); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing frozen rendition accepted", err)
	}
}

type mutableReadStore struct {
	asset  domain.MediaAsset
	err    error
	onFind func()
}

func (s *mutableReadStore) FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error) {
	if s.onFind != nil {
		s.onFind()
	}
	return s.asset, s.err
}
func (s *mutableReadStore) ListReadyAssets(context.Context, identityapp.Principal, uuid.UUID, string, uuid.UUID, int) ([]domain.MediaAsset, error) {
	return []domain.MediaAsset{s.asset}, s.err
}
func TestRoutedPreviewDoesNotPublishGrantWhenRevokedWhileSigning(t *testing.T) {
	a := routedReadImage()
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	store := &mutableReadStore{asset: a}
	objects := &routedReadObjects{grants: map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://fixture.invalid/private", ExpiresAt: time.Now().Add(40 * time.Second)}}, onSign: func() { store.err = identityapp.ErrForbidden }}
	result, err := mediaapp.NewRoutedAssetQuery(store, objects, time.Minute).Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID)
	if !errors.Is(err, identityapp.ErrForbidden) || result.URL != "" {
		t.Fatal("revocation during signing still published a new lease", err)
	}
}

type mutableLibraryReader struct {
	file mediaapp.LibraryMediaFile
	err  error
}

func (r *mutableLibraryReader) FindLibraryMedia(context.Context, identityapp.Principal, domain.LibraryScope, uuid.UUID) (mediaapp.LibraryMediaFile, error) {
	return r.file, r.err
}
func TestRoutedLibraryDoesNotPublishGrantWhenRevokedWhileSigning(t *testing.T) {
	a := routedReadImage()
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	reader := &mutableLibraryReader{file: mediaapp.LibraryMediaFile{Asset: a}}
	objects := &routedReadObjects{grants: map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://fixture.invalid/private", ExpiresAt: time.Now().Add(40 * time.Second)}}, onSign: func() { reader.err = identityapp.ErrForbidden }}
	result, err := mediaapp.NewRoutedLibraryMediaQuery(reader, objects, objects, time.Minute).Preview(t.Context(), identityapp.Principal{}, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &a.ProjectID}, a.ID)
	if !errors.Is(err, identityapp.ErrForbidden) || result.URL != "" {
		t.Fatal("library revocation during signing published a lease", err)
	}
}

func TestRoutedPreviewRejectsLeaseExpiringDuringFinalAuthorization(t *testing.T) {
	a := routedReadImage()
	a.ObjectLocation = readLocation("minio-main", a.ObjectKey)
	calls := 0
	store := &mutableReadStore{asset: a, onFind: func() {
		calls++
		if calls == 2 {
			time.Sleep(300 * time.Millisecond)
		}
	}}
	objects := &routedReadObjects{grants: map[domain.ObjectRef]mediaapp.AccessGrant{a.ObjectLocation.Ref: {URL: "https://fixture.invalid/private", ExpiresAt: time.Now().Add(time.Second)}}}
	objects.onSign = func() {
		objects.grants[a.ObjectLocation.Ref] = mediaapp.AccessGrant{URL: "https://fixture.invalid/private", ExpiresAt: time.Now().Add(200 * time.Millisecond)}
	}
	result, err := mediaapp.NewRoutedAssetQuery(store, objects, time.Minute).Preview(t.Context(), identityapp.Principal{}, a.ProjectID, a.ID)
	if calls != 2 {
		t.Fatal("expected final authority check")
	}
	if !errors.Is(err, mediaapp.ErrUnavailable) || result.URL != "" {
		t.Fatal("expired lease published after slow authority recheck", err)
	}
}
