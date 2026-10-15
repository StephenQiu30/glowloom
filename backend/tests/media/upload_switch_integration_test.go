package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func uploadRoutingPolicy() mediaapp.UploadPolicy {
	return mediaapp.UploadPolicy{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}
}

func activeTestStorageRoute(t *testing.T, routes *pgmedia.StorageRoutes) mediaapp.StorageRoute {
	t.Helper()
	route, err := routes.ReadActive(t.Context())
	if errors.Is(err, mediaapp.ErrStorageNotReady) {
		route, err = routes.Activate(t.Context(), "", "fixture-"+uuid.NewString(), "minio-main")
	}
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func TestUploadAdmissionPersistsAcrossRouteSwitch(t *testing.T) {
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	current := activeTestStorageRoute(t, routes)
	revision := "admission-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), current.Revision, revision, "minio-main"); err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, database)
	admission := pgmedia.NewUploadAdmission(routingMediaStore(database), uploadRoutingPolicy(), 24*time.Hour)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	key := uuid.New()
	first, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routes.Activate(t.Context(), revision, "admission-next-"+uuid.NewString(), "cos-main"); err != nil {
		t.Fatal(err)
	}
	retry, err := admission.PinUpload(t.Context(), actor, scope, key)
	if err != nil || retry != first {
		t.Fatal("in-flight target changed", err)
	}
	next, err := admission.PinUpload(t.Context(), actor, scope, uuid.New())
	if err != nil || next.ProfileID != "cos-main" || next.SessionID == first.SessionID {
		t.Fatal("new admission ignored active route", err)
	}
}

// Routing tests use the committed media authorization fixture.
func routingMediaStore(database *gorm.DB) *pgmedia.Store {
	return pgmedia.NewStore(database)
}

type persistedRoutedUpload struct {
	locations *pgmedia.StorageRoutes
	writes    int
}

func (o *persistedRoutedUpload) CreateUploadObject(ctx context.Context, target mediaapp.UploadTarget, key string, file *mediaapp.Downloaded) (domain.ObjectLocation, error) {
	location, err := o.locations.ReserveUploadLocation(ctx, target.SessionID, domain.ObjectRef{ProfileID: target.ProfileID, Bucket: target.Bucket, Key: key})
	if err != nil {
		return location, err
	}
	o.writes++
	info := mediaapp.ObjectInfo{Size: file.Size, ContentType: file.MIMEType, SHA256: file.SHA256}
	if err := o.locations.ConfirmUploadLocation(ctx, location, info); err != nil {
		return location, err
	}
	location.State = "present"
	return location, nil
}
func (o *persistedRoutedUpload) RemoveUploadObject(ctx context.Context, location domain.ObjectLocation) error {
	remove, err := o.locations.BeginUploadRemoval(ctx, location)
	if err != nil || !remove {
		return err
	}
	return o.locations.ConfirmUploadRemoval(ctx, location)
}

func TestUploadReplayKeepsOriginalLocation(t *testing.T) {
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	activeTestStorageRoute(t, routes)
	actor, project := mediaStoreProject(t, database)
	store := routingMediaStore(database)
	admission := pgmedia.NewUploadAdmission(store, uploadRoutingPolicy(), 24*time.Hour)
	objects := &persistedRoutedUpload{locations: routes}
	service := mediaapp.NewRoutedUploadService(store, store, admission, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, objects, time.Now)
	input := uploadInput(t)
	input.Actor = actor
	input.Request.ProjectID = project
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	target, err := service.BeginUpload(t.Context(), actor, scope, input.Request.Key)
	if err != nil {
		t.Fatal(err)
	}
	input.Target = &target
	first, err := service.Upload(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ ObjectLocationID uuid.UUID }
	if err := database.Raw(`SELECT object_location_id FROM media.media_asset WHERE id=?`, first.Asset.ID).Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReadLocation(t.Context(), saved.ObjectLocationID)
	if err != nil || location.Ref.ProfileID != target.ProfileID || location.State != "accepted" {
		t.Fatal("accepted upload lost physical identity", err)
	}
	remove, err := routes.BeginUploadRemoval(t.Context(), location)
	if err != nil || remove {
		t.Fatal("accepted object admitted to compensation", err)
	}
	if _, err := routes.Activate(t.Context(), target.ConfigRevision, "replay-"+uuid.NewString(), "minio-main"); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Upload(t.Context(), input)
	if err != nil || replay.Asset.ID != first.Asset.ID || objects.writes != 3 {
		t.Fatal("replay moved location or wrote objects", err)
	}
}

func TestUploadAcceptanceRejectsAnotherActorsAdmission(t *testing.T) {
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	activeTestStorageRoute(t, routes)
	owner, ownerProject := mediaStoreProject(t, database)
	actor, project := mediaStoreProject(t, database)
	store := routingMediaStore(database)
	admission := pgmedia.NewUploadAdmission(store, uploadRoutingPolicy(), 24*time.Hour)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &ownerProject}
	stolen, err := admission.PinUpload(t.Context(), owner, scope, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	input := uploadInput(t)
	input.Actor = actor
	input.Request.ProjectID = project
	input.Target = &stolen
	// Fault injection models a corrupted orchestration argument. Acceptance must
	// independently bind the session to this actor, scope and idempotency key.
	badAdmission := &routedAdmissionFake{targets: map[uuid.UUID]mediaapp.UploadTarget{input.Request.Key: stolen}}
	service := mediaapp.NewRoutedUploadService(store, store, badAdmission, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, &persistedRoutedUpload{locations: routes}, time.Now)
	if _, err := service.Upload(t.Context(), input); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatalf("foreign admission reached acceptance: %v", err)
	}
}

func TestUploadMissingRoutedDependencyDoesNotReadBody(t *testing.T) {
	service := mediaapp.NewRoutedUploadService(&uploadRepoFake{}, nil, &routedAdmissionFake{active: "minio-main"}, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, nil, time.Now)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()})
		c.Next()
	})
	mediahttp.NewUploadHandler(service).Register(group)
	body := &unreadUploadBody{}
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/media/uploads", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 503 || body.reads.Load() != 0 {
		t.Fatalf("missing storage dependency read body: %d / %d reads", response.Code, body.reads.Load())
	}
}

func TestStorageAdmissionSnapshotAndOwnerAreImmutable(t *testing.T) {
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	activeTestStorageRoute(t, routes)
	actor, project := mediaStoreProject(t, database)
	admission := pgmedia.NewUploadAdmission(routingMediaStore(database), uploadRoutingPolicy(), time.Hour)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	a, err := admission.PinUpload(t.Context(), actor, scope, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	b, err := admission.PinUpload(t.Context(), actor, scope, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveUploadLocation(t.Context(), a.SessionID, domain.ObjectRef{ProfileID: a.ProfileID, Bucket: a.Bucket, Key: "projects/" + project.String() + "/image/" + uuid.NewString() + ".png"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE media.upload_admission SET config_revision='changed' WHERE id=?`, a.SessionID).Error; err == nil {
		t.Error("admission snapshot was rebound")
	}
	if err := database.Exec(`UPDATE media.object_location SET owner_session_id=? WHERE id=?`, b.SessionID, location.ID).Error; err == nil {
		t.Error("object ownership was rebound")
	}
}

func TestUploadSwitchDuringBodyKeepsPersistentTarget(t *testing.T) {
	database := storageRouteDB(t)
	routes := pgmedia.NewStorageRoutes(database)
	storageProfiles(t, routes)
	current := activeTestStorageRoute(t, routes)
	rev := "http-pin-" + uuid.NewString()
	if _, err := routes.Activate(t.Context(), current.Revision, rev, "minio-main"); err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, database)
	store := routingMediaStore(database)
	admission := pgmedia.NewUploadAdmission(store, uploadRoutingPolicy(), 24*time.Hour)
	service := mediaapp.NewRoutedUploadService(store, store, admission, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, &persistedRoutedUpload{locations: routes}, time.Now)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewUploadHandler(service).Register(group)
	request := uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
	request.URL.Path = "/api/projects/" + project.String() + "/media/uploads"
	gate := &gateUploadBody{ReadCloser: request.Body, read: make(chan struct{}), release: make(chan struct{})}
	request.Body = gate
	done := make(chan struct{})
	response := httptest.NewRecorder()
	go func() { defer close(done); router.ServeHTTP(response, request) }()
	select {
	case <-gate.read:
	case <-time.After(5 * time.Second):
		close(gate.release)
		<-done
		t.Fatal("multipart read never began")
	}
	var count int64
	err := database.Raw(`SELECT count(*) FROM media.upload_admission WHERE actor_id=? AND request_key=? AND profile_id='minio-main'`, actor.ID, request.Header.Get("Idempotency-Key")).Scan(&count).Error
	if err != nil || count != 1 {
		close(gate.release)
		<-done
		t.Fatal("body read preceded durable admission", err)
	}
	_, err = routes.Activate(t.Context(), rev, "http-next-"+uuid.NewString(), "cos-main")
	close(gate.release)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != 201 {
		t.Fatalf("switched in-flight upload failed: %d %s", response.Code, response.Body.String())
	}
	var accepted int64
	if err := database.Raw(`SELECT count(*) FROM media.object_location l JOIN media.upload_admission u ON u.id=l.owner_session_id WHERE u.actor_id=? AND u.request_key=? AND l.profile_id='minio-main' AND l.state='accepted'`, actor.ID, request.Header.Get("Idempotency-Key")).Scan(&accepted).Error; err != nil || accepted != 3 {
		t.Fatal("original or preview moved after activation", accepted, err)
	}
}

type routedAdmissionFake struct {
	mu      sync.Mutex
	active  string
	targets map[uuid.UUID]mediaapp.UploadTarget
	pinned  chan struct{}
	once    sync.Once
}

func (a *routedAdmissionFake) PinUpload(_ context.Context, _ identityapp.Principal, _ domain.LibraryScope, key uuid.UUID) (mediaapp.UploadTarget, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.targets == nil {
		a.targets = map[uuid.UUID]mediaapp.UploadTarget{}
	}
	target, ok := a.targets[key]
	if !ok {
		target = mediaapp.UploadTarget{SessionID: uuid.New(), ProfileID: a.active, Bucket: "storage-test", ConfigRevision: a.active + "-v1", ExpiresAt: time.Now().Add(time.Hour), Policy: uploadRoutingPolicy()}
		a.targets[key] = target
	}
	if a.pinned != nil {
		a.once.Do(func() { close(a.pinned) })
	}
	return target, nil
}

type routedUploadFake struct {
	refs    []domain.ObjectRef
	removed []domain.ObjectRef
}

func (o *routedUploadFake) CreateUploadObject(_ context.Context, target mediaapp.UploadTarget, key string, file *mediaapp.Downloaded) (domain.ObjectLocation, error) {
	ref := domain.ObjectRef{ProfileID: target.ProfileID, Bucket: target.Bucket, Key: key}
	o.refs = append(o.refs, ref)
	return domain.ObjectLocation{ID: uuid.New(), Ref: ref, State: "present", ByteSize: &file.Size, SHA256: &file.SHA256, MIMEType: &file.MIMEType}, nil
}
func (o *routedUploadFake) RemoveUploadObject(_ context.Context, location domain.ObjectLocation) error {
	o.removed = append(o.removed, location.Ref)
	return nil
}

type gateUploadBody struct {
	io.ReadCloser
	once    sync.Once
	read    chan struct{}
	release chan struct{}
}

func (b *gateUploadBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.read); <-b.release })
	return b.ReadCloser.Read(p)
}

type routingImageProbe struct{}

func (routingImageProbe) Probe(context.Context, *mediaapp.Downloaded) (mediaapp.ProbeResult, error) {
	w, h := int32(3), int32(4)
	codec := "png"
	return mediaapp.ProbeResult{Kind: domain.KindImage, Extension: "png", Width: &w, Height: &h, Codec: &codec}, nil
}

type routingImageRenderer struct{ t *testing.T }

func (r routingImageRenderer) Render(ctx context.Context, _ *mediaapp.Downloaded, _ mediaapp.ProbeResult, _ string) ([]mediaapp.RenditionFile, error) {
	var out []mediaapp.RenditionFile
	for _, kind := range []domain.RenditionKind{domain.RenditionThumb256, domain.RenditionThumb640} {
		file, err := mediaapp.ReadUpload(ctx, bytes.NewReader(uploadPNG(r.t)), "thumb.png")
		if err != nil {
			return nil, err
		}
		out = append(out, mediaapp.RenditionFile{Kind: kind, Result: file, Width: 3, Height: 4, Ext: "png"})
	}
	return out, nil
}

func TestUploadPinsBeforeReadingBody(t *testing.T) {
	repo := &uploadRepoFake{}
	admission := &routedAdmissionFake{active: "minio-main", pinned: make(chan struct{})}
	objects := &routedUploadFake{}
	service := mediaapp.NewRoutedUploadService(repo, nil, admission, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, objects, time.Now)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()})
		c.Next()
	})
	mediahttp.NewUploadHandler(service).Register(group)
	request := uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
	gate := &gateUploadBody{ReadCloser: request.Body, read: make(chan struct{}), release: make(chan struct{})}
	request.Body = gate
	done := make(chan struct{})
	response := httptest.NewRecorder()
	go func() { defer close(done); router.ServeHTTP(response, request) }()
	select {
	case <-gate.read:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start reading")
	}
	select {
	case <-admission.pinned:
	default:
		close(gate.release)
		<-done
		t.Fatal("multipart body read before durable pin")
	}
	admission.mu.Lock()
	admission.active = "cos-main"
	admission.mu.Unlock()
	close(gate.release)
	<-done
	if response.Code != http.StatusCreated {
		t.Fatalf("upload response: %d %s", response.Code, response.Body.String())
	}
	if len(objects.refs) != 3 {
		t.Fatalf("expected original and 2 previews, got %d", len(objects.refs))
	}
	for _, ref := range objects.refs {
		if ref.ProfileID != "minio-main" {
			t.Fatal("in-flight object moved", ref.ProfileID)
		}
	}
	if repo.asset.ObjectLocation == nil || len(repo.rends) != 2 || repo.rends[0].ObjectLocation == nil {
		t.Fatal("accepted facts lost physical location")
	}
	next := uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
	nextResponse := httptest.NewRecorder()
	router.ServeHTTP(nextResponse, next)
	if nextResponse.Code != 201 || objects.refs[3].ProfileID != "cos-main" {
		t.Fatal("next upload did not use new route")
	}
}

func TestRoutedUploadCommitUnknownKeepsObjects(t *testing.T) {
	repo := &uploadRepoFake{commitErr: errors.New("lost commit reply"), existsErr: errors.New("database unavailable")}
	admission := &routedAdmissionFake{active: "minio-main"}
	objects := &routedUploadFake{}
	service := mediaapp.NewRoutedUploadService(repo, nil, admission, routingImageProbe{}, mediaflow.FFUploadNormalizer{}, routingImageRenderer{t}, objects, time.Now)
	input := uploadInput(t)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &input.Request.ProjectID}
	target, err := service.BeginUpload(t.Context(), input.Actor, scope, input.Request.Key)
	if err != nil {
		t.Fatal(err)
	}
	input.Target = &target
	if _, err := service.Upload(t.Context(), input); err == nil {
		t.Fatal("unknown commit reported successful")
	}
	if len(objects.removed) != 0 {
		t.Fatal("unknown commit destroyed potentially accepted objects")
	}
}
