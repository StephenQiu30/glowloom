package application

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// Service applies closed content actions inside one currently authorized owning transaction.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService injects the real repository and clock; no global seeds or provider execution are created.
func NewService(store Store, now func() time.Time) *Service { return &Service{store: store, now: now} }

// Change publishes a new immutable snapshot or an explicit head/confirmation transition.
func (s *Service) Change(ctx context.Context, actor identityapp.Principal, c Command) (Receipt, error) {
	if s == nil || s.store == nil || s.now == nil {
		return Receipt{}, ErrUnavailable
	}
	hash, err := c.Fingerprint()
	if err != nil {
		return Receipt{}, err
	}
	return s.store.Write(ctx, actor, c.ProjectID, func(tx Transaction) (Receipt, error) {
		prior, err := tx.Replay(ctx, actor, c, hash)
		if err != nil {
			return Receipt{}, err
		}
		if prior != nil {
			return *prior, nil
		}
		now := s.now().UTC().Truncate(time.Microsecond)
		var h domain.Head
		var v domain.Version
		var childHeadID, childVersionID *uuid.UUID
		creating := c.Action == "create" || c.Action == "create_result"
		if creating {
			h = domain.Head{ID: uuid.New(), OrgID: actor.OrgID, ProjectID: c.ProjectID, Kind: c.Kind, Revision: 1, CreatedAt: now, UpdatedAt: now}
			v = domain.Version{ID: uuid.New(), EntryID: h.ID, OrgID: h.OrgID, ProjectID: h.ProjectID, Kind: h.Kind, Number: 1, ActorID: actor.ID, CreatedAt: now, Origin: domain.OriginManual}
			if c.Action == "create" {
				if err := setContent(&v, c); err != nil {
					return Receipt{}, err
				}
			}
		} else {
			h, v, err = tx.Load(ctx, c.Kind, c.EntryID)
			if err != nil {
				return Receipt{}, err
			}
			if h.Revision != c.ExpectedRevision || h.Revision >= math.MaxInt32 || h.RedirectID != nil || (h.Deleted && c.Action != "restore") {
				return Receipt{}, ErrConflict
			}
			h.Revision++
			h.UpdatedAt = now
			if actionMakesVersion(c.Action) {
				previous := v.ID
				v.ID = uuid.New()
				v.PreviousID = &previous
				v.Number++
				v.ActorID = actor.ID
				v.CreatedAt = now
				v.Origin = domain.OriginManual
				v.Result = nil
			}
		}
		if creating || actionMakesVersion(c.Action) {
			if err := changeVersion(ctx, tx, actor, c, &v); err != nil {
				return Receipt{}, err
			}
			if err := validateDependencies(ctx, tx, actor, c.ProjectID, v); err != nil {
				return Receipt{}, err
			}
			_, v.ContentSHA256, err = v.Content()
			if err != nil {
				return Receipt{}, err
			}
			if err := v.Validate(); err != nil {
				return Receipt{}, err
			}
			h.CurrentVersionID = v.ID
			if creating {
				if err := tx.InsertHead(ctx, h); err != nil {
					return Receipt{}, err
				}
			}
			if err := tx.InsertVersion(ctx, v); err != nil {
				return Receipt{}, err
			}
		}
		switch c.Action {
		case "confirm":
			if err := validateDependencies(ctx, tx, actor, c.ProjectID, v); err != nil {
				return Receipt{}, err
			}
			h.ConfirmedVersionID = &v.ID
			if err := tx.Confirm(ctx, h.Kind, domain.Confirmation{ID: uuid.New(), EntryID: h.ID, VersionID: v.ID, Revision: h.Revision, ActorID: actor.ID, CreatedAt: now}); err != nil {
				return Receipt{}, err
			}
		case "delete":
			if err := applyImpact(ctx, tx, actor, c, v); err != nil {
				return Receipt{}, err
			}
			h.Deleted = true
		case "restore":
			if !h.Deleted {
				return Receipt{}, ErrConflict
			}
			h.Deleted = false
		case "merge":
			if err := mergeCharacter(ctx, tx, actor, c, &h, v, now); err != nil {
				return Receipt{}, err
			}
		case "split":
			child, version, err := splitCharacter(ctx, tx, actor, c, v, now)
			if err != nil {
				return Receipt{}, err
			}
			childHeadID = &child.ID
			childVersionID = &version.ID
		}
		if !creating {
			if err := tx.UpdateHead(ctx, h, c.ExpectedRevision); err != nil {
				return Receipt{}, err
			}
		}
		projectRevision, err := tx.Access().TouchContent(ctx, actor, c.ProjectID, tx.Project().Revision)
		if err != nil {
			return Receipt{}, err
		}
		result := Receipt{EntryID: h.ID, Kind: h.Kind, Revision: h.Revision, VersionID: v.ID, VersionNumber: v.Number, ConfirmedVersionID: h.ConfirmedVersionID, ProjectRevision: projectRevision, ContentSHA256: v.ContentSHA256}
		result.CreatedEntryID = childHeadID
		result.CreatedVersionID = childVersionID
		result.RedirectID = h.RedirectID
		if err := tx.Record(ctx, actor, c, hash, result, now); err != nil {
			return Receipt{}, err
		}
		return result, nil
	})
}

func actionMakesVersion(action string) bool {
	switch action {
	case "update", "look_create", "look_update", "look_delete", "look_default", "references", "voice_bind", "voice_unbind", "adopt_result":
		return true
	default:
		return false
	}
}

func setContent(v *domain.Version, c Command) error {
	switch c.Kind {
	case domain.KindCharacter:
		if c.Character == nil {
			return domain.ErrInvalidContent
		}
		i := c.Character
		v.Character = &domain.CharacterContent{Name: i.Name, Aliases: i.Aliases, Description: i.Description, Definition: i.Definition, Looks: []domain.LookContent{{ID: uuid.New(), Name: "默认造型", Default: true}}}
	case domain.KindLocation:
		if c.Location == nil {
			return domain.ErrInvalidContent
		}
		content := *c.Location
		v.Location = &content
	case domain.KindProp:
		if c.Prop == nil {
			return domain.ErrInvalidContent
		}
		content := *c.Prop
		v.Prop = &content
	default:
		return domain.ErrInvalidContent
	}
	return nil
}

func changeVersion(ctx context.Context, tx Transaction, actor identityapp.Principal, c Command, v *domain.Version) error {
	switch c.Action {
	case "create":
		return nil
	case "update":
		if c.Kind == domain.KindCharacter {
			i := c.Character
			v.Character.Name = i.Name
			v.Character.Aliases = i.Aliases
			v.Character.Description = i.Description
			v.Character.Definition = i.Definition
			return nil
		}
		return setContent(v, c)
	case "look_create":
		l := c.Look
		if l.Default {
			for i := range v.Character.Looks {
				v.Character.Looks[i].Default = false
			}
		}
		v.Character.Looks = append(v.Character.Looks, domain.LookContent{ID: uuid.New(), Name: l.Name, Description: l.Description, Default: l.Default, AppliesTo: l.AppliesTo})
		return nil
	case "look_update", "look_default", "look_delete", "references":
		index := slices.IndexFunc(v.Character.Looks, func(l domain.LookContent) bool { return l.ID == *c.LookID })
		if index < 0 {
			return ErrNotFound
		}
		switch c.Action {
		case "look_update":
			l := c.Look
			if l.Default {
				for i := range v.Character.Looks {
					v.Character.Looks[i].Default = false
				}
			}
			old := v.Character.Looks[index]
			v.Character.Looks[index] = domain.LookContent{ID: old.ID, Name: l.Name, Description: l.Description, Default: l.Default, AppliesTo: l.AppliesTo, References: old.References}
		case "look_default":
			for i := range v.Character.Looks {
				v.Character.Looks[i].Default = i == index
			}
		case "look_delete":
			if err := applyImpact(ctx, tx, actor, c, *v); err != nil {
				return err
			}
			v.Character.Looks = slices.Delete(v.Character.Looks, index, index+1)
		case "references":
			if tx.Media() == nil {
				return ErrUnavailable
			}
			refs := make([]domain.ImageReference, 0, len(c.References))
			for _, r := range c.References {
				if !r.Role.Valid() || r.AssetID == uuid.Nil {
					return domain.ErrInvalidContent
				}
				fact, err := tx.Media().Reference(ctx, actor, c.ProjectID, r.AssetID, "image")
				if err != nil {
					return err
				}
				if fact.AssetID != r.AssetID || fact.Validate("image") != nil {
					return ErrUnavailable
				}
				refs = append(refs, domain.ImageReference{Role: r.Role, Media: fact})
			}
			v.Character.Looks[index].References = refs
		}
		return nil
	case "voice_bind":
		input := c.Voice
		content := &domain.VoiceContent{Kind: input.Kind, Instructions: input.Instructions}
		switch input.Kind {
		case domain.VoiceSample:
			if input.Sample == nil || input.Catalog != nil || tx.Media() == nil {
				return domain.ErrInvalidContent
			}
			fact, err := tx.Media().Reference(ctx, actor, c.ProjectID, input.Sample.AssetID, "audio")
			if err != nil {
				return err
			}
			if fact.AssetID != input.Sample.AssetID {
				return ErrUnavailable
			}
			content.Sample = &domain.SampleVoice{Name: input.Sample.Name, Media: fact}
		case domain.VoiceCatalog:
			if input.Catalog == nil || input.Sample != nil {
				return domain.ErrInvalidContent
			}
			if tx.Voices() == nil {
				return ErrUnavailable
			}
			fact, err := tx.Voices().Reference(ctx, actor, c.ProjectID, *input.Catalog)
			if err != nil {
				return err
			}
			if fact.ModelKey != input.Catalog.ModelKey || fact.ModelVersion != input.Catalog.ExpectedModelVersion || fact.VoiceKey != input.Catalog.VoiceKey || !fact.Params.Equal(input.Catalog.Params) {
				return ErrUnavailable
			}
			content.Catalog = &fact
		default:
			return domain.ErrInvalidContent
		}
		v.Character.Voice = content
		return nil
	case "voice_unbind":
		v.Character.Voice = nil
		return nil
	case "adopt_result", "create_result":
		if tx.Results() == nil {
			return ErrUnavailable
		}
		var source domain.ResultSource
		var err error
		switch v.Kind {
		case domain.KindCharacter:
			var content CharacterInput
			content, source, err = tx.Results().Character(ctx, actor, c.ProjectID, *c.OperationID, *c.OutputID)
			if err == nil {
				if c.Action == "create_result" {
					generated := c
					generated.Character = &content
					err = setContent(v, generated)
				} else {
					v.Character.Name = content.Name
					v.Character.Aliases = content.Aliases
					v.Character.Description = content.Description
					v.Character.Definition = content.Definition
				}
			}
		case domain.KindLocation:
			var content domain.LocationContent
			content, source, err = tx.Results().Location(ctx, actor, c.ProjectID, *c.OperationID, *c.OutputID)
			v.Location = &content
		case domain.KindProp:
			var content domain.PropContent
			content, source, err = tx.Results().Prop(ctx, actor, c.ProjectID, *c.OperationID, *c.OutputID)
			v.Prop = &content
		}
		if err != nil {
			return err
		}
		if source.OperationID != *c.OperationID || source.OutputID != *c.OutputID {
			return ErrUnavailable
		}
		v.Origin = domain.OriginAI
		v.Result = &source
		return nil
	default:
		return domain.ErrInvalidContent
	}
}

func validateDependencies(ctx context.Context, tx Transaction, actor identityapp.Principal, project uuid.UUID, v domain.Version) error {
	if v.Character == nil {
		return nil
	}
	for _, look := range v.Character.Looks {
		if len(look.AppliesTo) > 0 {
			if tx.Scopes() == nil {
				return ErrUnavailable
			}
			if err := tx.Scopes().ValidateScopes(ctx, actor, project, look.AppliesTo); err != nil {
				return err
			}
		}
		for _, ref := range look.References {
			if tx.Media() == nil {
				return ErrUnavailable
			}
			if err := tx.Media().Verify(ctx, actor, project, ref.Media); err != nil {
				return err
			}
		}
	}
	voice := v.Character.Voice
	if voice == nil {
		return nil
	}
	if voice.Kind == domain.VoiceSample {
		if tx.Media() == nil {
			return ErrUnavailable
		}
		return tx.Media().Verify(ctx, actor, project, voice.Sample.Media)
	}
	if tx.Voices() == nil {
		return ErrUnavailable
	}
	c := voice.Catalog
	fact, err := tx.Voices().Reference(ctx, actor, project, VoiceSelection{ModelKey: c.ModelKey, ExpectedModelVersion: c.ModelVersion, VoiceKey: c.VoiceKey, Params: c.Params})
	if err != nil {
		return err
	}
	if !fact.Equal(*c) {
		return ErrConflict
	}
	return nil
}

func applyImpact(ctx context.Context, tx Transaction, actor identityapp.Principal, c Command, v domain.Version) error {
	if tx.Impacts() == nil {
		return ErrUnavailable
	}
	input := ImpactInput{Kind: c.Kind, EntryID: v.EntryID, VersionID: v.ID, Action: c.Action, LookID: c.LookID}
	if v.PreviousID != nil && actionMakesVersion(c.Action) {
		input.VersionID = *v.PreviousID
	}
	proof, err := tx.Impacts().Read(ctx, actor, c.ProjectID, input)
	if err != nil {
		return err
	}
	if c.AcknowledgedImpact == nil || *c.AcknowledgedImpact != proof {
		return &ImpactConflict{Proof: proof}
	}
	return tx.Impacts().Apply(ctx, actor, c.ProjectID, input, proof)
}

// ImpactConflict asks for explicit review of actual frozen downstream evidence.
type ImpactConflict struct{ Proof ImpactProof }

func (e *ImpactConflict) Error() string { return "Bible impact acknowledgment required" }
func (e *ImpactConflict) Unwrap() error { return ErrConflict }

// Find reads an identity and resolves only the current confirmed redirect chain.
func (s *Service) Find(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, id uuid.UUID) (Detail, error) {
	if s == nil || s.store == nil {
		return Detail{}, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil || !kind.Valid() {
		return Detail{}, domain.ErrInvalidContent
	}
	return s.store.Find(ctx, actor, project, kind, id)
}

// Version reads a pinned immutable version; a redirect never changes its identity.
func (s *Service) Version(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, id, version uuid.UUID) (domain.Version, error) {
	if s == nil || s.store == nil {
		return domain.Version{}, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil || version == uuid.Nil || !kind.Valid() {
		return domain.Version{}, domain.ErrInvalidContent
	}
	return s.store.Version(ctx, actor, project, kind, id, version)
}

// List returns a bounded project-scoped page without writes or seeds.
func (s *Service) List(ctx context.Context, actor identityapp.Principal, input ListInput) (Page, error) {
	if s == nil || s.store == nil {
		return Page{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || !input.Kind.Valid() || input.Limit < 0 || input.Limit > 200 {
		return Page{}, domain.ErrInvalidContent
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	return s.store.List(ctx, actor, input)
}

// History reads all immutable content through bounded pages.
func (s *Service) History(ctx context.Context, actor identityapp.Principal, project uuid.UUID, kind domain.Kind, id uuid.UUID, limit int, after *HistoryCursor) (HistoryPage, error) {
	if s == nil || s.store == nil {
		return HistoryPage{}, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil || !kind.Valid() || limit < 0 || limit > 50 {
		return HistoryPage{}, domain.ErrInvalidContent
	}
	if limit == 0 {
		limit = 20
	}
	return s.store.History(ctx, actor, project, kind, id, limit, after)
}

// Voices lists real configured choices; missing catalog context is explicit.
func (s *Service) Voices(ctx context.Context, actor identityapp.Principal, project uuid.UUID, limit int, after *VoiceCursor) (VoicePage, error) {
	if s == nil || s.store == nil {
		return VoicePage{}, ErrUnavailable
	}
	if project == uuid.Nil || limit < 0 || limit > 200 {
		return VoicePage{}, domain.ErrInvalidContent
	}
	if limit == 0 {
		limit = 50
	}
	return s.store.Voices(ctx, actor, project, limit, after)
}
