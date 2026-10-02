package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Source and input limits are independent from original-document file budgets.
const (
	MaxSourceTitleScalars = 512
	MaxChapterImport      = 2500
)

// ErrInvalidSource rejects incomplete or duplicated source composition facts.
var ErrInvalidSource = errors.New("invalid script source")

// SourceDocument contains immutable source identity and its checked private content.
// It is an internal preparation model, not a database or public response DTO.
type SourceDocument struct {
	ID         uuid.UUID    `json:"id"`
	LineageID  uuid.UUID    `json:"source_lineage_id"`
	PreviousID *uuid.UUID   `json:"previous_source_id,omitempty"`
	Revision   int64        `json:"source_revision"`
	Kind       string       `json:"source_kind"`
	Title      string       `json:"title"`
	Status     string       `json:"status"`
	Document   RichDocument `json:"document"`
}

// Validate checks source metadata without interpreting editor status as confirmation.
func (s SourceDocument) Validate() error {
	if s.ID == uuid.Nil || s.LineageID == uuid.Nil || s.Revision < 1 || s.PreviousID != nil && *s.PreviousID == uuid.Nil || !slices.Contains([]string{"chapter", "episode", "document"}, s.Kind) || !slices.Contains([]string{"draft", "ready", "completed"}, s.Status) || strings.TrimSpace(s.Title) == "" || !utf8.ValidString(s.Title) || strings.ContainsRune(s.Title, 0) || utf8.RuneCountInString(s.Title) > MaxSourceTitleScalars {
		return ErrInvalidSource
	}
	return s.Document.Validate()
}

// SourceSpan binds an ordered source snapshot to canonical version scalar coordinates.
type SourceSpan struct {
	SourceID  uuid.UUID `json:"source_id"`
	LineageID uuid.UUID `json:"source_lineage_id"`
	Position  int       `json:"position"`
	Start     int       `json:"start"`
	End       int       `json:"end"`
}

// ComposedVersion contains all actual content, hashes and nonempty source candidates.
type ComposedVersion struct {
	Text           string
	RichJSON       []byte
	CharCount      int
	ContentHash    string
	DocumentSHA256 string
	ManifestSHA256 string
	Spans          []SourceSpan
	Candidates     []EpisodeBoundary
}

// ComposeSources preserves the complete source order and separates semantic hashes.
// Delimiter scalars belong to the preceding nonempty episode. Empty editor sources
// remain in Spans but never become zero-width parse candidates.
func ComposeSources(sources []SourceDocument) (ComposedVersion, error) {
	result := ComposedVersion{Spans: make([]SourceSpan, 0, len(sources)), Candidates: make([]EpisodeBoundary, 0)}
	documents := make([]RichDocument, 0, len(sources))
	seenIDs := make(map[uuid.UUID]bool, len(sources))
	seenLineages := make(map[uuid.UUID]bool, len(sources))
	var text strings.Builder
	lastNonempty := -1
	for i, source := range sources {
		if err := source.Validate(); err != nil {
			return ComposedVersion{}, fmt.Errorf("source %d: %w", i, err)
		}
		if seenIDs[source.ID] || seenLineages[source.LineageID] {
			return ComposedVersion{}, ErrInvalidSource
		}
		seenIDs[source.ID], seenLineages[source.LineageID] = true, true
		plain, err := source.Document.PlainText()
		if err != nil {
			return ComposedVersion{}, err
		}
		if plain != "" && lastNonempty >= 0 {
			text.WriteString("\n\n")
			result.CharCount += 2
			result.Spans[lastNonempty].End += 2
		}
		start := result.CharCount
		text.WriteString(plain)
		result.CharCount += utf8.RuneCountInString(plain)
		if result.CharCount > MaxScalarCount {
			return ComposedVersion{}, ErrInvalidDocument
		}
		result.Spans = append(result.Spans, SourceSpan{SourceID: source.ID, LineageID: source.LineageID, Position: i, Start: start, End: result.CharCount})
		if plain != "" {
			lastNonempty = i
		}
		if strings.TrimSpace(plain) != "" {
			candidateStart := start
			if len(result.Candidates) == 0 {
				candidateStart = 0
			} else {
				result.Candidates[len(result.Candidates)-1].End = start
			}
			result.Candidates = append(result.Candidates, EpisodeBoundary{SeqNo: len(result.Candidates) + 1, Title: source.Title, Start: candidateStart, End: result.CharCount, SourceLineageID: &source.LineageID})
		}
		documents = append(documents, source.Document)
	}
	result.Text = text.String()
	if len(result.Candidates) > 0 {
		result.Candidates[len(result.Candidates)-1].End = result.CharCount
	}
	rich, err := json.Marshal(documents)
	if err != nil || len(rich) > MaxVersionBytes {
		return ComposedVersion{}, ErrInvalidDocument
	}
	manifest, err := json.Marshal(sources)
	if err != nil || len(manifest) > MaxHTTPBytes {
		return ComposedVersion{}, ErrInvalidSource
	}
	result.RichJSON = rich
	result.ContentHash, result.DocumentSHA256, result.ManifestSHA256 = ContentSHA([]byte(result.Text)), ContentSHA(rich), ContentSHA(manifest)
	return result, nil
}
