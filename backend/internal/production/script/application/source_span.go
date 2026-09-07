package application

import (
	"context"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type SourceSpanQuery struct {
	ProjectID, DocumentRevisionID, ExpectedHash string
	Start, End                                  int
}

// ReadSpan returns evidence from the specified accepted version, never the current head.
func (service *SourceService) ReadSpan(ctx context.Context, actor Actor, query SourceSpanQuery) (domain.SourceSpan, error) {
	_, hashErr := hex.DecodeString(query.ExpectedHash)
	if query.Start < 0 || query.End <= query.Start || query.End-query.Start > 16384 || len(query.ExpectedHash) != 64 || hashErr != nil || query.ExpectedHash != strings.ToLower(query.ExpectedHash) {
		return domain.SourceSpan{}, invalid("Invalid source span or expected hash")
	}
	var result domain.SourceSpan
	err := service.transactions.WithinSourceTransaction(ctx, func(repo SourceRepository) error {
		workspaceID, err := repo.ProjectWorkspace(ctx, actor, query.ProjectID, false)
		if err != nil {
			return err
		}
		accepted, err := repo.GetAcceptedSource(ctx, query.ProjectID, query.DocumentRevisionID)
		if err != nil {
			return err
		}
		analysis, err := repo.GetAnalysis(ctx, query.DocumentRevisionID)
		if err != nil {
			return err
		}
		if analysis.Document.ProjectID != query.ProjectID || analysis.Document.WorkspaceID != workspaceID {
			return ErrNotFound
		}
		text := analysis.Revision.NormalizedText
		if !utf8.ValidString(text) || accepted.Identity.VersionID != analysis.Revision.ID || accepted.Identity.LogicalID != analysis.Document.ID ||
			accepted.Identity.ContentHash != query.ExpectedHash || analysis.Revision.NormalizedHash != query.ExpectedHash || textHash(text) != query.ExpectedHash ||
			accepted.CodepointCount != utf8.RuneCountInString(text) || accepted.CodepointIndexRule != "unicode-code-point" {
			return sourceError("source_hash_drift", "Source version or evidence index changed", 409)
		}
		if query.End > accepted.CodepointCount {
			return invalid("Source span exceeds source length")
		}
		fragment := string([]rune(text)[query.Start:query.End])
		result = domain.SourceSpan{Identity: accepted.Identity, SpanIndexID: accepted.SpanIndexID, Start: query.Start, End: query.End, Text: fragment, TextHash: textHash(fragment), CodepointIndexRule: accepted.CodepointIndexRule}
		return nil
	})
	return result, normalizeError(err)
}
