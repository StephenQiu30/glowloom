package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AssetMutationFactory binds lifecycle owners to the enclosing command transaction.
type AssetMutationFactory func(*gorm.DB) application.AssetMutationAccess

// AssetCommandStore keeps personal preferences and existing lifecycle receipts atomic.
type AssetCommandStore struct {
	db     *gorm.DB
	access AssetMutationFactory
}

// NewAssetCommandStore injects the database and transaction-bound owning-module ports.
func NewAssetCommandStore(database *gorm.DB, access AssetMutationFactory) *AssetCommandStore {
	return &AssetCommandStore{db: database, access: access}
}

func assetCommandFacts(tx *gorm.DB, actor identity.Principal, id uuid.UUID, write bool) (application.AssetMutationFacts, error) {
	var facts application.AssetMutationFacts
	lock := " FOR SHARE OF p,d"
	if write {
		lock = " FOR UPDATE OF p,d"
	}
	read := tx.Raw(`SELECT d.id,d.project_id,d.revision,p.revision AS project_revision,
 COALESCE(p.initial_canvas_id=d.id,false) AS is_main,d.asset_collection AS collection
 FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id
 WHERE d.id=? AND p.org_id=? AND p.owner_scope_id=? AND NOT d.is_delete
 AND NOT p.is_delete AND p.status IN ('active','archived')`+lock, id, actor.OrgID, actor.ScopeID).Scan(&facts)
	if read.Error != nil {
		return facts, read.Error
	}
	if read.RowsAffected != 1 {
		return facts, application.ErrNotFound
	}
	return facts, nil
}

func favoriteAsset(tx *gorm.DB, actor identity.Principal, facts application.AssetMutationFacts, item application.AssetCommandItem, favorite bool) (application.AssetCommandResult, error) {
	result := application.AssetCommandResult{ID: item.ID, Revision: application.Revision(facts.Revision), ProjectRevision: facts.ProjectRevision}
	var locked int
	if err := tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69361,hashtext(?))`, actor.ID.String()+":"+actor.ScopeID.String()+":"+item.ID.String()).Scan(&locked).Error; err != nil {
		return result, err
	}
	var current struct {
		Favorite bool
		Revision int64
	}
	read := tx.Raw(`SELECT favorite,revision FROM canvas.asset_preference WHERE actor_id=? AND scope_id=? AND document_id=? FOR UPDATE`, actor.ID, actor.ScopeID, item.ID).Scan(&current)
	if read.Error != nil {
		return result, read.Error
	}
	result.Favorite, result.FavoriteRevision = current.Favorite, application.Revision(current.Revision)
	if current.Revision != int64(item.ExpectedFavoriteRevision) {
		return result, &application.AssetActionError{Code: "favorite_revision_conflict"}
	}
	if current.Favorite == favorite {
		return result, nil
	}
	if current.Revision == math.MaxInt64 {
		return result, domain.ErrInvalidCommand
	}
	current.Revision++
	if err := tx.Exec(`INSERT INTO canvas.asset_preference(actor_id,scope_id,document_id,favorite,revision)
 VALUES(?,?,?,?,?) ON CONFLICT(actor_id,scope_id,document_id)
 DO UPDATE SET favorite=excluded.favorite,revision=excluded.revision,updated_at=now()`, actor.ID, actor.ScopeID, item.ID, favorite, current.Revision).Error; err != nil {
		return result, err
	}
	result.Favorite, result.FavoriteRevision = favorite, application.Revision(current.Revision)
	return result, nil
}

func assetFailureCode(err error) string {
	var action *application.AssetActionError
	var revision *application.RevisionConflict
	switch {
	case errors.As(err, &action):
		return action.Code
	case errors.As(err, &revision):
		return "revision_conflict"
	case errors.Is(err, application.ErrNotFound):
		return "not_found"
	case errors.Is(err, identity.ErrForbidden):
		return "forbidden"
	case errors.Is(err, application.ErrMainCanvasRequired):
		return "main_canvas_required"
	case errors.Is(err, application.ErrMainCanvasManagedByProject):
		return "main_canvas_managed_by_project"
	case errors.Is(err, domain.ErrGraphReadOnly):
		return "canvas_graph_read_only"
	case errors.Is(err, domain.ErrInvalidCommand):
		return "invalid_request"
	default:
		return ""
	}
}

// ApplyAssetCommand stores a frozen partial receipt with all successful mutations.
// Unknown database/dependency failures abort the entire transaction, not just an item.
func (s *AssetCommandStore) ApplyAssetCommand(ctx context.Context, actor identity.Principal, key uuid.UUID, input application.AssetCommandInput) (application.AssetCommandReceipt, error) {
	result := application.AssetCommandReceipt{}
	if s == nil || s.db == nil || s.access == nil || key == uuid.Nil {
		return result, domain.ErrInvalidCommand
	}
	if err := input.Validate(); err != nil {
		return result, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := s.access(tx)
		if access == nil {
			return application.ErrNotFound
		}
		write := input.Action != "favorite"
		if err := access.AuthorizeAssetCommand(ctx, actor, write); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		hash := requestHash("canvas-assets/"+actor.OrgID.String()+"/"+actor.ScopeID.String(), body)
		found, err := replay(tx, actor.ID, key.String(), hash, &result)
		if err != nil || found {
			return err
		}
		result.Results = make([]application.AssetCommandResult, len(input.Items))
		indices := make([]int, len(input.Items))
		main := make(map[uuid.UUID]bool)
		for index, item := range input.Items {
			indices[index] = index
			var isMain bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project WHERE initial_canvas_id=? AND org_id=? AND owner_scope_id=?)`, item.ID, actor.OrgID, actor.ScopeID).Scan(&isMain).Error; err != nil {
				return err
			}
			main[item.ID] = isMain
		}
		// Use one deterministic lock order across batch requests. Selected secondary
		// documents precede mains so project recycling cannot erase their outcome.
		sort.Slice(indices, func(i, j int) bool {
			left, right := input.Items[indices[i]].ID, input.Items[indices[j]].ID
			if input.Action == "delete" && main[left] != main[right] {
				return !main[left]
			}
			return left.String() < right.String()
		})
		for _, index := range indices {
			item := input.Items[index]
			name := fmt.Sprintf("canvas_asset_%d", index)
			if err := tx.SavePoint(name).Error; err != nil {
				return err
			}
			entry := application.AssetCommandResult{ID: item.ID}
			facts, err := assetCommandFacts(tx, actor, item.ID, write)
			if err == nil {
				entry.Revision, entry.ProjectRevision = application.Revision(facts.Revision), facts.ProjectRevision
				// Recycling a main entry is a workspace lifecycle action. It may
				// delete an archived project without enabling archived content edits.
				projectWrite := write && (input.Action != "delete" || !facts.IsMain)
				err = access.AuthorizeAssetProject(ctx, actor, facts.ProjectID, projectWrite)
			}
			if err == nil && write && facts.Revision != int64(item.ExpectedRevision) {
				err = &application.RevisionConflict{CurrentRevision: facts.Revision}
			}
			if err == nil {
				itemKey := uuid.NewSHA1(key, []byte(input.Action+"/"+item.ID.String()))
				switch input.Action {
				case "favorite":
					entry, err = favoriteAsset(tx, actor, facts, item, *input.Favorite)
				case "rename":
					entry, err = access.RenameAsset(ctx, actor, facts, itemKey, item, input.Name)
				case "delete":
					entry, err = access.DeleteAsset(ctx, actor, facts, itemKey, item)
				}
			}
			if err != nil {
				code := assetFailureCode(err)
				if code == "" {
					return err
				}
				if err := tx.RollbackTo(name).Error; err != nil {
					return err
				}
				entry.Status, entry.Code = "failed", code
				result.Failed++
			} else {
				entry.Status = "succeeded"
				result.Succeeded++
			}
			entry.ID = item.ID
			result.Results[index] = entry
		}
		return receipt(tx, actor.ID, key.String(), hash, 200, result)
	})
	return result, err
}
