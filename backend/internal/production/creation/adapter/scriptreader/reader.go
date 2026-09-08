package scriptreader

import (
	"context"
	"errors"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type SourceService interface {
	ReadText(context.Context, scriptapp.Actor, scriptapp.SourceTextQuery) (scriptdomain.SourceText, error)
}

type Reader struct{ source SourceService }

func New(source SourceService) *Reader { return &Reader{source: source} }

func (reader *Reader) ReadFrozenSource(ctx context.Context, actor app.Actor, projectID string, source domain.Source) (domain.FrozenSource, error) {
	value, err := reader.source.ReadText(ctx, scriptapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, scriptapp.SourceTextQuery{ProjectID: projectID, DocumentRevisionID: source.RevisionID, ExpectedHash: source.ContentHash})
	if err != nil {
		var problem *scriptapp.Error
		if errors.Is(err, scriptapp.ErrNotFound) {
			return domain.FrozenSource{}, app.ErrNotFound
		}
		if errors.As(err, &problem) {
			return domain.FrozenSource{}, app.Problem(problem.Code, problem.Status)
		}
		return domain.FrozenSource{}, err
	}
	identity := value.Accepted.Identity
	if identity.LogicalID != source.DocumentID || identity.VersionID != source.RevisionID || identity.Revision != source.Revision || identity.ContentHash != source.ContentHash || value.Accepted.SpanIndexID != source.SpanIndexID {
		return domain.FrozenSource{}, app.Problem("source_hash_drift", 409)
	}
	return domain.FrozenSource{RevisionID: identity.VersionID, ContentHash: identity.ContentHash, Text: value.Text}, nil
}
