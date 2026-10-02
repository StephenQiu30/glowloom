package script_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func TestScriptCompositionFullSourcesScalarSpansAndHashes(t *testing.T) {
	first, err := extract.HTML(`<p>😀é</p>`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := extract.HTML(`<p>中文</p>`)
	if err != nil {
		t.Fatal(err)
	}
	blank, err := extract.HTML(`<p></p>`)
	if err != nil {
		t.Fatal(err)
	}
	sources := []domain.SourceDocument{
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "一", Status: "ready", Revision: 1, Document: first},
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "空", Status: "draft", Revision: 1, Document: blank},
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "episode", Title: "二", Status: "ready", Revision: 1, Document: second},
	}
	composed, err := domain.ComposeSources(sources)
	if err != nil || composed.Text != "😀é\n\n中文" || composed.CharCount != 7 {
		t.Fatalf("composition/scalar error: %+v %v", composed, err)
	}
	if composed.Spans[0].Start != 0 || composed.Spans[0].End != 5 || composed.Spans[1].Start != 3 || composed.Spans[1].End != 3 || composed.Spans[2].Start != 5 || composed.Spans[2].End != 7 {
		t.Fatalf("separator ownership or zero-width source: %+v", composed.Spans)
	}
	if len(composed.Candidates) != 2 || composed.Candidates[0].SeqNo != 1 || composed.Candidates[1].SeqNo != 2 || composed.Candidates[0].End != composed.Candidates[1].Start {
		t.Fatalf("source0 became episode0 or candidate gap: %+v", composed.Candidates)
	}
	sources[0].Title = "新标题"
	renamed, err := domain.ComposeSources(sources)
	if err != nil || renamed.ContentHash != composed.ContentHash || renamed.DocumentSHA256 != composed.DocumentSHA256 || renamed.ManifestSHA256 == composed.ManifestSHA256 {
		t.Fatalf("metadata erased or mixed text/rich/manifest hashes: %v", err)
	}
}

func TestScriptCompositionRejectsDuplicateLineageAndInvalidTitle(t *testing.T) {
	doc, err := extract.HTML(`<p>x</p>`)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.SourceDocument{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "x", Status: "draft", Revision: 1, Document: doc}
	if _, err := domain.ComposeSources([]domain.SourceDocument{source, source}); !errors.Is(err, domain.ErrInvalidSource) {
		t.Fatalf("accepted duplicate source lineage: %v", err)
	}
	source.Title = " "
	if _, err := domain.ComposeSources([]domain.SourceDocument{source}); !errors.Is(err, domain.ErrInvalidSource) {
		t.Fatalf("accepted empty source title: %v", err)
	}
}

func TestScriptRulesSplitPrefaceAndUnicodeCoordinates(t *testing.T) {
	text := "人物😀\n\n第１集 雨\n甲é\n第2集 晴\n乙"
	result, err := domain.RulesSplit(text)
	if err != nil || result.Preface == nil || result.Preface.Start != 0 || result.Preface.End != 5 || len(result.Episodes) != 2 {
		t.Fatalf("preface or rules: %+v %v", result, err)
	}
	if result.Episodes[0].SeqNo != 1 || result.Episodes[1].SeqNo != 2 || result.Episodes[0].End != result.Episodes[1].Start || result.Episodes[1].End != len([]rune(text)) {
		t.Fatalf("incomplete or non-scalar rules spans: %+v", result)
	}
	fragment, err := domain.ScalarSlice(text, result.Episodes[0].Start, result.Episodes[0].End)
	if err != nil || fragment != "第１集 雨\n甲é\n" {
		t.Fatalf("normalized original was rewritten: %q %v", fragment, err)
	}
	if err := domain.ValidateBoundaries(result.Episodes, result.Preface, len([]rune(text))); err != nil {
		t.Fatal(err)
	}
	result.Episodes[1].Start++
	if err := domain.ValidateBoundaries(result.Episodes, result.Preface, len([]rune(text))); !errors.Is(err, domain.ErrInvalidSpan) {
		t.Fatal("accepted boundary gap")
	}
}

func TestScriptRulesEmptyAndFallback(t *testing.T) {
	result, err := domain.RulesSplit("未标记的😀正文")
	if err != nil || len(result.Episodes) != 1 || result.Episodes[0].Start != 0 || result.Episodes[0].End != 7 {
		t.Fatalf("fallback missing complete content: %+v %v", result, err)
	}
	empty, err := domain.RulesSplit(" \n\t")
	if err != nil || len(empty.Episodes) != 0 {
		t.Fatalf("blank created parseable episode: %+v %v", empty, err)
	}
}

func TestScriptWhitespacePartitionsPreserveEveryScalar(t *testing.T) {
	doc, err := extract.HTML(`<p></p><p></p>`)
	if err != nil {
		t.Fatal(err)
	}
	text, err := extract.HTML(`<p>x</p>`)
	if err != nil {
		t.Fatal(err)
	}
	sources := []domain.SourceDocument{
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "空", Status: "draft", Revision: 1, Document: doc},
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "一", Status: "ready", Revision: 1, Document: text},
		{ID: uuid.New(), LineageID: uuid.New(), Kind: "chapter", Title: "空", Status: "draft", Revision: 1, Document: doc},
	}
	composition, err := domain.ComposeSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateBoundaries(composition.Candidates, nil, composition.CharCount); err != nil {
		t.Fatalf("blank source left a scalar gap: %+v %v", composition, err)
	}
	input := " \n第1集\n甲\n第2集\n乙"
	result, err := domain.RulesSplit(input)
	if err != nil || len(result.Episodes) != 2 {
		t.Fatalf("leading whitespace rules failed: %+v %v", result, err)
	}
	part, err := domain.ScalarSlice(input, result.Episodes[1].Start, result.Episodes[1].End)
	if err != nil || part != "第2集\n乙" {
		t.Fatalf("heading recognition changed absolute coordinates: %q %v", part, err)
	}
}
