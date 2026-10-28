package subject_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func TestSubjectSaveIsAtomicAndReplaysOnlyAfterAuthorization(t *testing.T) {
	ctx := context.Background()
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	service := application.NewService(repo, func() time.Time { return time.Unix(1800000000, 0) })
	primary, secondary := fact("image"), fact("image")
	repo.media.facts[primary.AssetID] = primary
	input := application.SaveInput{Name: "原子主体", Primary: application.MediaSelection{AssetID: primary.AssetID}, References: []application.MediaSelection{{AssetID: secondary.AssetID}}}
	key := uuid.New()
	if _, err := service.Save(ctx, actor, uuid.Nil, key, input); err == nil {
		t.Fatal("saved missing supplement")
	}
	if len(repo.heads) != 0 || len(repo.receipts) != 0 {
		t.Fatal("partial subject or receipt committed")
	}
	repo.media.facts[secondary.AssetID] = secondary
	saved, err := service.Save(ctx, actor, uuid.Nil, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.References) != 1 || saved.Primary.AssetID != primary.AssetID {
		t.Fatal("incomplete settings")
	}
	repo.media.fail = true
	replay, err := service.Save(ctx, actor, uuid.Nil, key, input)
	if err != nil || replay.ID != saved.ID {
		t.Fatalf("same key did not replay: %v", err)
	}
	input.Name = "different"
	if _, err := service.Save(ctx, actor, uuid.Nil, key, input); !errors.Is(err, application.ErrKeyConflict) {
		t.Fatalf("same key different body: %v", err)
	}
	repo.denied = true
	input.Name = "原子主体"
	if _, err := service.Save(ctx, actor, uuid.Nil, key, input); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("replayed without current authority: %v", err)
	}
}

func TestSubjectCASAndFavoritesDoNotRewriteContent(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	media := fact("video")
	repo.media.facts[media.AssetID] = media
	service := application.NewService(repo, func() time.Time { return time.Unix(1800000000, 0) })
	saved, err := service.Save(context.Background(), actor, uuid.Nil, uuid.New(), application.SaveInput{Name: "视频主体", Primary: application.MediaSelection{AssetID: media.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	before := repo.versions[saved.VersionID]
	result, err := service.Favorite(context.Background(), actor, saved.ID, uuid.New(), application.FavoriteInput{ExpectedRevision: 1, Favorite: true})
	if err != nil || result.Favorite == nil || !*result.Favorite {
		t.Fatalf("favorite failed: %v", err)
	}
	if len(repo.versions) != 1 || repo.heads[saved.ID].Revision != 1 || repo.versions[saved.VersionID].SHA256 != before.SHA256 {
		t.Fatal("favorite rewrote shared content")
	}
	_, err = service.Save(context.Background(), actor, saved.ID, uuid.New(), application.SaveInput{ExpectedRevision: 2, Name: "stale", Primary: application.MediaSelection{AssetID: media.AssetID}})
	if !errors.Is(err, application.ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	batch, err := service.Batch(context.Background(), actor, uuid.New(), application.BatchInput{Action: "delete", Items: []application.BatchItem{{ID: saved.ID, ExpectedRevision: 1}, {ID: uuid.New(), ExpectedRevision: 1}}})
	if err != nil || batch.Items[0].Status != "succeeded" || batch.Items[1].Code != "not_found" {
		t.Fatalf("incorrect batch outcomes: %#v %v", batch, err)
	}
	if len(repo.versions) != 1 || repo.heads[saved.ID].DeletedAt == nil {
		t.Fatal("delete lost history or did not mark head")
	}
}

// The fake models commit/rollback, not SQL syntax. Real authority always precedes replay.
type memoryMedia struct {
	facts map[uuid.UUID]domain.MediaFact
	fail  bool
}

func (m *memoryMedia) Reference(_ context.Context, _ identity.Principal, s application.MediaSelection) (domain.MediaFact, error) {
	if m.fail {
		return domain.MediaFact{}, application.ErrUnavailable
	}
	f, ok := m.facts[s.AssetID]
	if !ok {
		return f, application.ErrNotFound
	}
	return f, nil
}

type memoryRepository struct {
	recordFailure bool
	actor         identity.Principal
	denied        bool
	media         *memoryMedia
	heads         map[uuid.UUID]domain.Head
	versions      map[uuid.UUID]domain.Version
	receipts      map[uuid.UUID]application.StoredReceipt
	favorites     map[uuid.UUID]bool
}

func newMemoryRepository(actor identity.Principal) *memoryRepository {
	return &memoryRepository{actor: actor, media: &memoryMedia{facts: map[uuid.UUID]domain.MediaFact{}}, heads: map[uuid.UUID]domain.Head{}, versions: map[uuid.UUID]domain.Version{}, receipts: map[uuid.UUID]application.StoredReceipt{}, favorites: map[uuid.UUID]bool{}}
}
func (r *memoryRepository) Write(_ context.Context, a identity.Principal, f func(application.Transaction) (application.CommandResult, error)) (application.CommandResult, error) {
	if r.denied || a.ScopeID != r.actor.ScopeID {
		return application.CommandResult{}, application.ErrNotFound
	}
	clone := *r
	clone.heads = copyMap(r.heads)
	clone.versions = copyMap(r.versions)
	clone.receipts = copyMap(r.receipts)
	clone.favorites = copyMap(r.favorites)
	result, err := f(&clone)
	if err == nil {
		*r = clone
	}
	return result, err
}
func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
func (r *memoryRepository) Scope() application.ScopeFacts {
	return application.ScopeFacts{ScopeID: r.actor.ScopeID, PhysicalScopeID: r.actor.OrgID}
}
func (r *memoryRepository) Media() application.MediaAccess { return r.media }
func (r *memoryRepository) Replay(_ context.Context, key uuid.UUID, hash string) (*application.CommandResult, error) {
	s, ok := r.receipts[key]
	if !ok {
		return nil, nil
	}
	if s.Hash != hash {
		return nil, application.ErrKeyConflict
	}
	return &s.Result, nil
}
func (r *memoryRepository) Find(_ context.Context, id uuid.UUID) (domain.Head, domain.Version, bool, error) {
	h, ok := r.heads[id]
	if !ok || h.DeletedAt != nil {
		return h, domain.Version{}, false, application.ErrNotFound
	}
	return h, r.versions[h.VersionID], r.favorites[id], nil
}
func (r *memoryRepository) Publish(_ context.Context, h domain.Head, v domain.Version, _ int64) error {
	r.heads[h.ID] = h
	r.versions[v.ID] = v
	return nil
}
func (r *memoryRepository) Delete(_ context.Context, h domain.Head, _ int64) error {
	r.heads[h.ID] = h
	return nil
}
func (r *memoryRepository) SetFavorite(_ context.Context, id uuid.UUID, value bool) error {
	r.favorites[id] = value
	return nil
}
func (r *memoryRepository) Record(_ context.Context, key uuid.UUID, hash string, result application.CommandResult) error {
	if r.recordFailure {
		return application.ErrUnavailable
	}
	r.receipts[key] = application.StoredReceipt{Hash: hash, Result: result}
	return nil
}

func TestSubjectReusesPinnedSourceWithoutRequiringOriginalOwnerSelection(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	media := fact("image")
	project := uuid.New()
	media.SourceProjectID = &project
	media.PersonalOrgID = nil
	media.PersonalActorID = nil
	repo.media.facts[media.AssetID] = media
	service := application.NewService(repo, time.Now)
	input := application.SaveInput{Name: "已采纳项目素材", Primary: application.MediaSelection{AssetID: media.AssetID}}
	if _, err := service.Save(context.Background(), actor, uuid.Nil, uuid.New(), input); !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("new source silently guessed project owner: %v", err)
	}
	binding := uuid.New()
	input.Primary.BindingID = &binding
	result, err := service.Save(context.Background(), actor, uuid.Nil, uuid.New(), input)
	if err != nil || result.Primary.AssetID != media.AssetID {
		t.Fatalf("owner-authorized pinned reuse failed: %v", err)
	}
}

func TestSubjectReceiptFailureRollsBackPublishedContent(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New(), OrgID: uuid.New()}
	repo := newMemoryRepository(actor)
	media := fact("image")
	repo.media.facts[media.AssetID] = media
	repo.recordFailure = true
	service := application.NewService(repo, time.Now)
	_, err := service.Save(context.Background(), actor, uuid.Nil, uuid.New(), application.SaveInput{Name: "回执失败", Primary: application.MediaSelection{AssetID: media.AssetID}})
	if !errors.Is(err, application.ErrUnavailable) || len(repo.heads) != 0 || len(repo.versions) != 0 || len(repo.receipts) != 0 {
		t.Fatalf("receipt failure left partial content: %v", err)
	}
}
