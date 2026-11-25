package postgres

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	skill "github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
)

func validResource(p string) bool {
	return p != "" && p != "." && !strings.ContainsAny(p, "\\\x00") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
func skillPackage(in domain.SkillInput) (*skill.SkillPackage, error) {
	if in.RequestID == uuid.Nil || in.SkillID != nil && *in.SkillID == uuid.Nil || len(in.Body) > 262144 || len(in.Resources) > 256 || len(in.Version) > 100 || in.Version == "" || len(in.Description) > 4000 {
		return nil, domain.ErrInvalid
	}
	total := len(in.Body)
	for name, content := range in.Resources {
		if !validResource(name) || len(name) > 1024 || len(content) > 262144 {
			return nil, domain.ErrInvalid
		}
		total += len(content)
	}
	if total > 1<<20 {
		return nil, domain.ErrInvalid
	}
	p, err := skill.NewSkillPackage(in.Name, in.Version, in.Description, in.Body, in.Resources)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	return p, nil
}

// InstallSkill records a private immutable skill version under current project access.
func (s *Store) InstallSkill(ctx context.Context, actor identity.Principal, pid uuid.UUID, in domain.SkillInput) (domain.SkillVersion, error) {
	p, err := skillPackage(in)
	if err != nil {
		return domain.SkillVersion{}, err
	}
	id := uuid.NewSHA1(actor.ScopeID, []byte("skill:"+actor.ID.String()+":"+in.RequestID.String()))
	if in.SkillID != nil {
		id = *in.SkillID
	}
	out := domain.SkillVersion{SkillID: id, Name: p.Name, Version: p.Version, Description: p.Description, ResourceHash: p.Hash, Enabled: true}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.request(tx, actor, pid, true); err != nil {
			return err
		}
		if in.SkillID != nil {
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM agent_business.skill_version WHERE skill_id=? AND owner_actor_id=? AND org_id=? AND scope_id=?`, id, actor.ID, actor.OrgID, actor.ScopeID).Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return domain.ErrNotFound
			}
		}
		resources, err := json.Marshal(p.Resources)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO agent_business.skill_version(skill_id,version,resource_hash,owner_actor_id,org_id,scope_id,name,description,body,resources,request_id) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb,?) ON CONFLICT DO NOTHING`, id, p.Version, p.Hash, actor.ID, actor.OrgID, actor.ScopeID, p.Name, p.Description, p.SkillMD, string(resources), in.RequestID).Error; err != nil {
			return err
		}
		var row struct {
			SkillID               uuid.UUID
			ResourceHash, Version string
			Enabled               bool
		}
		q := tx.Raw(`SELECT skill_id,resource_hash,version,enabled FROM agent_business.skill_version WHERE owner_actor_id=? AND org_id=? AND scope_id=? AND request_id=?`, actor.ID, actor.OrgID, actor.ScopeID, in.RequestID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || row.SkillID != id || row.Version != p.Version || row.ResourceHash != p.Hash {
			return domain.ErrConflict
		}
		out.Enabled = row.Enabled
		return nil
	})
	return out, err
}

// ListSkills lists the caller's private skill versions in the project.
func (s *Store) ListSkills(ctx context.Context, actor identity.Principal, pid uuid.UUID) ([]domain.SkillVersion, error) {
	out := []domain.SkillVersion{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.request(tx, actor, pid, false); err != nil {
			return err
		}
		return tx.Raw(`SELECT skill_id,name,version,description,resource_hash,enabled FROM agent_business.skill_version WHERE owner_actor_id=? AND org_id=? AND scope_id=? ORDER BY name,version,skill_id LIMIT 200`, actor.ID, actor.OrgID, actor.ScopeID).Scan(&out).Error
	})
	return out, err
}

// DiscoverSkills returns skill versions available to the current execution grant.
func (s *Store) DiscoverSkills(ctx context.Context, g domain.Grant) ([]domain.SkillVersion, error) {
	out := []domain.SkillVersion{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, g.RootRunID)
		if err != nil {
			return err
		}
		trusted, err := s.validate(tx, a, g)
		if err != nil {
			return err
		}
		scope := trusted.Scope
		return tx.Raw(`SELECT skill_id,name,version,description,resource_hash,enabled FROM agent_business.skill_version WHERE owner_actor_id=? AND org_id=? AND scope_id=? AND enabled ORDER BY name,version,skill_id LIMIT 200`, scope.ActorID, scope.OrgID, scope.ScopeID).Scan(&out).Error
	})
	return out, err
}

// SetSkillEnabled changes private skill availability under current project access.
func (s *Store) SetSkillEnabled(ctx context.Context, actor identity.Principal, pid, id uuid.UUID, version string, enabled bool) (domain.SkillVersion, error) {
	var out domain.SkillVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.request(tx, actor, pid, true); err != nil {
			return err
		}
		q := tx.Raw(`UPDATE agent_business.skill_version SET enabled=? WHERE skill_id=? AND version=? AND owner_actor_id=? AND org_id=? AND scope_id=? RETURNING skill_id,name,version,description,resource_hash,enabled`, enabled, id, version, actor.ID, actor.OrgID, actor.ScopeID).Scan(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		return nil
	})
	return out, err
}

// ReadSkill reads one pinned skill resource under the current grant.
func (s *Store) ReadSkill(ctx context.Context, g domain.Grant, id uuid.UUID, version, hash, resource string) (domain.SkillContent, error) {
	if resource != "" && !validResource(resource) {
		return domain.SkillContent{}, domain.ErrInvalid
	}
	var out domain.SkillContent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, g.RootRunID)
		if err != nil {
			return err
		}
		trusted, err := s.validate(tx, a, g)
		if err != nil {
			return err
		}
		scope := trusted.Scope
		pinned := false
		for _, pin := range a.SkillPins {
			if pin.SkillID == id && pin.Version == version && pin.ResourceHash == hash {
				pinned = true
				break
			}
		}
		if !pinned {
			return domain.ErrConflict
		}
		var row struct {
			SkillID                                        uuid.UUID
			Name, Version, Description, ResourceHash, Body string
			Resources                                      []byte
			Enabled                                        bool
		}
		q := tx.Raw(`SELECT * FROM agent_business.skill_version WHERE skill_id=? AND version=? AND owner_actor_id=? AND org_id=? AND scope_id=? AND enabled FOR SHARE`, id, version, scope.ActorID, scope.OrgID, scope.ScopeID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		if row.ResourceHash != hash {
			return domain.ErrConflict
		}
		var bundleResources map[string]string
		if json.Unmarshal(row.Resources, &bundleResources) != nil {
			return domain.ErrInvalid
		}
		bundle, err := skill.NewSkillPackage(row.Name, row.Version, row.Description, row.Body, bundleResources)
		if err != nil || bundle.Hash != row.ResourceHash {
			return domain.ErrConflict
		}
		out = domain.SkillContent{SkillVersion: domain.SkillVersion{SkillID: row.SkillID, Name: row.Name, Version: row.Version, Description: row.Description, ResourceHash: row.ResourceHash, Enabled: true}, ResourcePath: resource, Content: row.Body}
		if resource != "" {
			var resources map[string]string
			if json.Unmarshal(row.Resources, &resources) != nil {
				return domain.ErrInvalid
			}
			text, ok := resources[resource]
			if !ok {
				return domain.ErrNotFound
			}
			out.Content = text
		}
		return nil
	})
	return out, err
}

func validatePins(tx *gorm.DB, scope domain.Scope, pins []domain.SkillPin) error {
	for _, pin := range pins {
		var row struct{ SkillID uuid.UUID }
		q := tx.Raw(`SELECT skill_id FROM agent_business.skill_version WHERE skill_id=? AND version=? AND resource_hash=? AND owner_actor_id=? AND org_id=? AND scope_id=? AND enabled FOR SHARE`, pin.SkillID, pin.Version, pin.ResourceHash, scope.ActorID, scope.OrgID, scope.ScopeID).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrBlocked
		}
	}
	return nil
}

// ValidatePinnedSkills retains active version locks through model dispatch
// admission, so disabling a previously loaded Skill cannot open another action.
func ValidatePinnedSkills(tx *gorm.DB, run uuid.UUID, scope domain.Scope) error {
	raw, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	var row struct{ SkillPins []byte }
	q := tx.Raw(`SELECT skill_pins FROM agent_business.run_authorization WHERE root_run_id=? AND scope=?::jsonb`, run, string(raw)).Scan(&row)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	var pins []domain.SkillPin
	if json.Unmarshal(row.SkillPins, &pins) != nil {
		return domain.ErrInvalid
	}
	return validatePins(tx, scope, pins)
}
