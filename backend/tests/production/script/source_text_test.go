package script_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type textSourceStore struct {
	app.SourceRepository
	analysis domain.Analysis
	accepted domain.AcceptedSource
	index    domain.SourceSpanIndex
	revoked  bool
	write    bool
}

func (s *textSourceStore) WithinSourceTransaction(ctx context.Context, fn func(app.SourceRepository) error) error {
	return fn(s)
}
func (s *textSourceStore) ProjectWorkspace(_ context.Context, _ app.Actor, project string, write bool) (string, error) {
	s.write = write
	if s.revoked || project != s.analysis.Document.ProjectID {
		return "", &app.Error{Code: "forbidden", Status: 403}
	}
	return s.analysis.Document.WorkspaceID, nil
}
func (s *textSourceStore) GetAnalysis(context.Context, string) (domain.Analysis, error) {
	return s.analysis, nil
}
func (s *textSourceStore) GetAcceptedSource(context.Context, string, string) (domain.AcceptedSource, error) {
	return s.accepted, nil
}
func (s *textSourceStore) FindSpanIndex(context.Context, string) (domain.SourceSpanIndex, error) {
	return s.index, nil
}

func sourceTextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
func textSourceFixture(text string) (*textSourceStore, app.SourceTextQuery) {
	hash := sourceTextHash(text)
	store := &textSourceStore{
		analysis: domain.Analysis{Document: domain.Document{ID: "document", ProjectID: "project", WorkspaceID: "workspace"}, Revision: domain.Revision{ID: "revision", DocumentID: "document", WorkspaceID: "workspace", VersionNo: 1, RawText: "unrelated raw CRLF\r\n", NormalizedText: text, NormalizedHash: hash, CodepointCount: utf8.RuneCountInString(text)}},
		accepted: domain.AcceptedSource{Identity: domain.SourceVersionIdentity{OwnerKind: "production/script", LogicalID: "document", VersionID: "revision", Revision: 1, ContentHash: hash}, SpanIndexID: "index", SpanIndexHash: "index-hash", CodepointCount: utf8.RuneCountInString(text), UTF8ByteCount: len(text), NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point"},
		index:    domain.SourceSpanIndex{ID: "index", WorkspaceID: "workspace", ProjectID: "project", DocumentRevisionID: "revision", SourceHash: hash, ContentHash: "index-hash", CodepointCount: utf8.RuneCountInString(text), UTF8ByteCount: len(text), NewlineNormalization: "lf", CodepointIndexRule: "unicode-code-point"},
	}
	return store, app.SourceTextQuery{ProjectID: "project", DocumentRevisionID: "revision", ExpectedHash: hash}
}

func TestSourceTextPreservesAcceptedUnicodeAndNeverReturnsRawText(t *testing.T) {
	text := "甲😀e\u0301\n第二场。"
	store, query := textSourceFixture(text)
	service := app.NewSourceService(store, app.SourceConfig{})
	for range 2 {
		got, err := service.ReadText(context.Background(), app.Actor{}, query)
		if err != nil || got.Text != text || got.Accepted.Identity.ContentHash != sourceTextHash(text) || !store.write {
			t.Fatalf("accepted source changed: %+v, %v, write=%v", got, err, store.write)
		}
	}
	store.revoked = true
	if _, err := service.ReadText(context.Background(), app.Actor{}, query); app.ErrorCode(err) != "forbidden" {
		t.Fatalf("revoked source exposed: %v", err)
	}
}

func TestSourceTextRejectsDriftAndForeignSources(t *testing.T) {
	for name, mutate := range map[string]func(*textSourceStore, *app.SourceTextQuery){
		"expected hash":     func(_ *textSourceStore, q *app.SourceTextQuery) { q.ExpectedHash = strings.Repeat("f", 64) },
		"foreign document":  func(s *textSourceStore, _ *app.SourceTextQuery) { s.analysis.Document.ProjectID = "foreign" },
		"revision identity": func(s *textSourceStore, _ *app.SourceTextQuery) { s.analysis.Revision.ID = "foreign" },
		"text hash":         func(s *textSourceStore, _ *app.SourceTextQuery) { s.analysis.Revision.NormalizedText = "changed" },
		"raw hash paired with normalized": func(s *textSourceStore, q *app.SourceTextQuery) {
			q.ExpectedHash = sourceTextHash(s.analysis.Revision.RawText)
		},
		"index count":   func(s *textSourceStore, _ *app.SourceTextQuery) { s.index.CodepointCount++ },
		"index source":  func(s *textSourceStore, _ *app.SourceTextQuery) { s.index.SourceHash = strings.Repeat("f", 64) },
		"foreign index": func(s *textSourceStore, _ *app.SourceTextQuery) { s.index.ProjectID = "foreign" },
		"byte count":    func(s *textSourceStore, _ *app.SourceTextQuery) { s.accepted.UTF8ByteCount++ },
		"CR text":       func(s *textSourceStore, _ *app.SourceTextQuery) { s.analysis.Revision.NormalizedText = "甲\r乙" },
	} {
		t.Run(name, func(t *testing.T) {
			s, q := textSourceFixture("甲😀\n乙")
			mutate(s, &q)
			if _, err := app.NewSourceService(s, app.SourceConfig{}).ReadText(context.Background(), app.Actor{}, q); err == nil {
				t.Fatal("inconsistent source exposed")
			}
		})
	}
}

func TestSourceTextRejectsOverLimitInsteadOfTruncating(t *testing.T) {
	s, q := textSourceFixture(strings.Repeat("😀", 2_000_001))
	if _, err := app.NewSourceService(s, app.SourceConfig{}).ReadText(context.Background(), app.Actor{}, q); app.ErrorCode(err) != "source_too_large" {
		t.Fatalf("oversized source: %v", err)
	}
}
