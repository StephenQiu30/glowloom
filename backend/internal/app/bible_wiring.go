package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	pgscript "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

func provideBibleProjectAccess(tx *gorm.DB) bibleapp.ProjectAccess {
	return pgworkspace.NewProjectContentAccessStore(tx)
}

func provideBibleStore(database *gorm.DB, storage *objectstorage.Client, validator catalogapp.ModelVersionValidator) *pgbible.Store {
	return pgbible.NewStore(database, pgbible.Factories{
		Access: provideBibleProjectAccess,
		Media: func(tx *gorm.DB) bibleapp.MediaReferences {
			return bibleMediaReferences{query: mediaapp.NewReferenceFactQuery(
				pgmedia.NewLibraryStore(tx, provideMediaLibraryProjectAccess, time.Now), storage)}
		},
		Scopes: func(tx *gorm.DB) bibleapp.ScriptScopes {
			return bibleScriptScopes{query: pgscript.NewBibleScopes(tx, provideScriptProjectAccess(tx))}
		},
		Voices: func(tx *gorm.DB) bibleapp.CatalogVoices {
			return bibleCatalogVoices{query: catalogapp.NewVoiceCatalog(pgcatalog.NewStore(tx), validator)}
		},
	})
}

type bibleCatalogVoices struct{ query *catalogapp.VoiceCatalog }

func (v bibleCatalogVoices) Reference(ctx context.Context, actor identityapp.Principal, project uuid.UUID, selection bibleapp.VoiceSelection) (bibledomain.CatalogVoice, error) {
	params, err := json.Marshal(selection.Params)
	if err != nil {
		return bibledomain.CatalogVoice{}, errors.Join(bibledomain.ErrInvalidContent, err)
	}
	fact, err := v.query.Reference(ctx, actor, project, catalogapp.VoiceSelection{
		ModelKey: selection.ModelKey, ExpectedVersion: selection.ExpectedModelVersion, VoiceKey: selection.VoiceKey, Params: params,
	})
	if err != nil {
		return bibledomain.CatalogVoice{}, bibleVoiceError(err)
	}
	var provided bibledomain.VoiceParams
	if err := json.Unmarshal(fact.Params, &provided); err != nil || !provided.Equal(selection.Params) {
		return bibledomain.CatalogVoice{}, errors.Join(bibleapp.ErrUnavailable, err)
	}
	return bibledomain.CatalogVoice{ModelKey: fact.ModelKey, ModelVersionID: fact.ModelVersionID, ModelVersion: fact.ModelVersion, VoiceKey: fact.VoiceKey, ParamSchemaSHA256: fact.ParamSchemaSHA256, Params: provided}, nil
}

func (v bibleCatalogVoices) List(ctx context.Context, actor identityapp.Principal, project uuid.UUID, limit int, after *bibleapp.VoiceCursor) (bibleapp.VoicePage, error) {
	var cursor *catalogapp.VoiceCursor
	if after != nil {
		cursor = &catalogapp.VoiceCursor{ModelKey: after.ModelKey, VoiceKey: after.VoiceKey}
	}
	page, err := v.query.List(ctx, actor, project, limit, cursor)
	if err != nil {
		return bibleapp.VoicePage{}, bibleVoiceError(err)
	}
	result := bibleapp.VoicePage{Voices: make([]bibleapp.VoiceChoice, len(page.Voices))}
	for i, choice := range page.Voices {
		result.Voices[i] = bibleapp.VoiceChoice{ModelKey: choice.ModelKey, ModelVersion: choice.ModelVersion, VoiceKey: choice.VoiceKey, DisplayName: choice.DisplayName}
	}
	if page.Next != nil {
		result.Next = &bibleapp.VoiceCursor{ModelKey: page.Next.ModelKey, VoiceKey: page.Next.VoiceKey}
	}
	return result, nil
}

func bibleVoiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, catalogapp.ErrVoiceModelNotFound), errors.Is(err, catalogapp.ErrModelCatalogProjectNotFound):
		return errors.Join(bibleapp.ErrNotFound, err)
	case errors.Is(err, catalogapp.ErrVoiceVersionConflict):
		return errors.Join(bibleapp.ErrConflict, err)
	case errors.Is(err, catalogapp.ErrInvalidVoiceSelection):
		return errors.Join(bibledomain.ErrInvalidContent, err)
	default:
		return errors.Join(bibleapp.ErrUnavailable, err)
	}
}

// The caller transaction retains every current authorization and media lock
// until the Bible write and all bounded private object reads have finished.
type bibleMediaReferences struct{ query *mediaapp.ReferenceFactQuery }

func (m bibleMediaReferences) Reference(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID, kind string) (bibledomain.MediaFact, error) {
	fact, err := m.query.Reference(ctx, actor, projectID, assetID, mediadomain.Kind(kind))
	if err != nil {
		return bibledomain.MediaFact{}, bibleMediaError(err)
	}
	result := bibledomain.MediaFact{AssetID: fact.AssetID, Revision: fact.Revision, Kind: string(fact.Kind), SHA256: fact.SHA256, ByteSize: fact.ByteSize}
	if fact.RenditionID != nil {
		result.RenditionID = *fact.RenditionID
	}
	if fact.RenditionSHA256 != nil {
		result.RenditionSHA256 = *fact.RenditionSHA256
	}
	if err := result.Validate(kind); err != nil {
		return bibledomain.MediaFact{}, errors.Join(bibleapp.ErrUnavailable, err)
	}
	return result, nil
}

func (m bibleMediaReferences) Verify(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, fact bibledomain.MediaFact) error {
	if err := fact.Validate(fact.Kind); err != nil {
		return errors.Join(bibleapp.ErrUnavailable, err)
	}
	input := mediaapp.ReferenceFact{AssetID: fact.AssetID, Revision: fact.Revision, Kind: mediadomain.Kind(fact.Kind), SHA256: fact.SHA256, ByteSize: fact.ByteSize}
	if fact.Kind == string(mediadomain.KindImage) {
		input.RenditionID, input.RenditionSHA256 = &fact.RenditionID, &fact.RenditionSHA256
	}
	return bibleMediaError(m.query.VerifyFrozenReference(ctx, actor, projectID, input))
}

func bibleMediaError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, mediaapp.ErrNotFound):
		return errors.Join(bibleapp.ErrNotFound, err)
	case errors.Is(err, mediaapp.ErrLibraryConflict), errors.Is(err, mediadomain.ErrMediaStateConflict):
		return errors.Join(bibleapp.ErrConflict, err)
	case errors.Is(err, mediaapp.ErrInvalidQuery):
		return errors.Join(bibledomain.ErrInvalidContent, err)
	default:
		return errors.Join(bibleapp.ErrUnavailable, err)
	}
}

type bibleScriptScopes struct{ query *pgscript.BibleScopes }

func (s bibleScriptScopes) ValidateScopes(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, scopes []bibledomain.LookScope) error {
	input := make([]scriptapp.BibleLookScope, len(scopes))
	for i, scope := range scopes {
		input[i] = scriptapp.BibleLookScope{EpisodeID: scope.EpisodeID, SceneKey: scope.SceneKey}
	}
	err := s.query.ValidateLookScopes(ctx, actor, projectID, input)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, scriptapp.ErrNotFound):
		return errors.Join(bibleapp.ErrNotFound, err)
	case errors.Is(err, scriptapp.ErrConflict):
		return errors.Join(bibleapp.ErrConflict, err)
	case errors.Is(err, scriptdomain.ErrInvalidStructure):
		return errors.Join(bibledomain.ErrInvalidContent, err)
	default:
		return errors.Join(bibleapp.ErrUnavailable, err)
	}
}

type scriptCharacterReferences struct {
	reader bibleapp.CharacterReferenceReader
}

func provideScriptCharacterReferences(tx *gorm.DB) scriptapp.CharacterReferences {
	return scriptCharacterReferences{reader: pgbible.NewReferences(tx, provideBibleProjectAccess(tx))}
}

func (c scriptCharacterReferences) Reference(ctx context.Context, actor identityapp.Principal, projectID, characterID uuid.UUID, pinnedVersionID *uuid.UUID) (scriptapp.CharacterReference, error) {
	fact, err := c.reader.Reference(ctx, actor, projectID, characterID, pinnedVersionID)
	switch {
	case err == nil:
		return scriptapp.CharacterReference{CharacterID: fact.CharacterID, VersionID: fact.VersionID, Revision: fact.Revision}, nil
	case errors.Is(err, identityapp.ErrForbidden), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return scriptapp.CharacterReference{}, err
	case errors.Is(err, bibleapp.ErrNotFound):
		return scriptapp.CharacterReference{}, errors.Join(scriptapp.ErrNotFound, err)
	case errors.Is(err, bibleapp.ErrConflict):
		return scriptapp.CharacterReference{}, errors.Join(scriptapp.ErrConflict, err)
	default:
		return scriptapp.CharacterReference{}, errors.Join(scriptapp.ErrContextUnavailable, err)
	}
}
