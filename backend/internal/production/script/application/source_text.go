package application

import (
	"context"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

const MaxSourceCodepoints = 2_000_000

type SourceTextQuery struct {
	ProjectID, DocumentRevisionID, ExpectedHash string
}

// ReadText returns the accepted LF representation used by source hashes and evidence
// offsets. RawText is a different representation and must never accompany this hash.
func (service *SourceService) ReadText(ctx context.Context, actor Actor, query SourceTextQuery) (domain.SourceText, error) {
	_, hashErr := hex.DecodeString(query.ExpectedHash)
	if len(query.ExpectedHash) != 64 || hashErr != nil || query.ExpectedHash != strings.ToLower(query.ExpectedHash) {
		return domain.SourceText{}, invalid("Invalid expected source hash")
	}
	var result domain.SourceText
	err := service.transactions.WithinSourceTransaction(ctx, func(repo SourceRepository) error {
		workspaceID, err := repo.ProjectWorkspace(ctx, actor, query.ProjectID, true)
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
		index, err := repo.FindSpanIndex(ctx, query.DocumentRevisionID)
		if err != nil {
			return err
		}
		text := analysis.Revision.NormalizedText
		count := utf8.RuneCountInString(text)
		if count > MaxSourceCodepoints {
			return sourceError("source_too_large", "Accepted source exceeds the execution limit", 413)
		}
		if text == "" || !utf8.ValidString(text) || strings.Contains(text, "\r") ||
			analysis.Revision.ID != query.DocumentRevisionID || analysis.Revision.DocumentID != analysis.Document.ID || analysis.Revision.WorkspaceID != workspaceID ||
			accepted.Identity.OwnerKind != "production/script" || accepted.Identity.VersionID != query.DocumentRevisionID || accepted.Identity.LogicalID != analysis.Document.ID || accepted.Identity.Revision != int64(analysis.Revision.VersionNo) ||
			accepted.Identity.ContentHash != query.ExpectedHash || analysis.Revision.NormalizedHash != query.ExpectedHash || textHash(text) != query.ExpectedHash ||
			accepted.SpanIndexID != index.ID || accepted.SpanIndexHash != index.ContentHash || index.DocumentRevisionID != query.DocumentRevisionID || index.SourceHash != query.ExpectedHash || index.ProjectID != query.ProjectID || index.WorkspaceID != workspaceID ||
			accepted.CodepointCount != count || analysis.Revision.CodepointCount != count || index.CodepointCount != count || accepted.UTF8ByteCount != len(text) || index.UTF8ByteCount != len(text) ||
			accepted.CodepointIndexRule != "unicode-code-point" || index.CodepointIndexRule != "unicode-code-point" || accepted.NewlineNormalization != "lf" || index.NewlineNormalization != "lf" {
			return sourceError("source_hash_drift", "Accepted source text or code-point index changed", 409)
		}
		result = domain.SourceText{Accepted: accepted, Text: text}
		return nil
	})
	return result, normalizeError(err)
}
