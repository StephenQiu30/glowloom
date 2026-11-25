package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// ScalarSpan refers exclusively to canonical version Unicode-scalar offsets.
type ScalarSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// EpisodeBoundary is an editable proposed partition, not a confirmed episode fact.
type EpisodeBoundary struct {
	SeqNo           int        `json:"seq_no"`
	Title           string     `json:"title"`
	Start           int        `json:"span_start"`
	End             int        `json:"span_end"`
	SourceLineageID *uuid.UUID `json:"source_lineage_id,omitempty"`
}

// SplitResult preserves optional preface context and all episode source spans.
type SplitResult struct {
	Preface  *ScalarSpan       `json:"preface,omitempty"`
	Episodes []EpisodeBoundary `json:"episodes"`
	Warnings []string          `json:"warnings,omitempty"`
}

var episodeHeading = regexp.MustCompile(`(?i)^(第\s*[0-9零〇一二三四五六七八九十百千两]+\s*集|EP(?:ISODE)?\s*[0-9]+)(?:[\s:：.、\-—].*)?$`)

// RulesSplit recognizes complete episode heading lines without rewriting source text.
// NFKC is used only for heading recognition; spans and titles retain original text.
func RulesSplit(text string) (SplitResult, error) {
	result := SplitResult{Episodes: make([]EpisodeBoundary, 0)}
	if !utf8.ValidString(text) || text != NormalizeText(text) || strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) > MaxScalarCount {
		return result, ErrInvalidDocument
	}
	if strings.TrimSpace(text) == "" {
		return result, nil
	}
	position := 0
	for _, line := range strings.Split(text, "\n") {
		title := strings.TrimSpace(line)
		if utf8.RuneCountInString(title) <= 120 && episodeHeading.MatchString(norm.NFKC.String(title)) {
			headingStart := position
			if len(result.Episodes) > 0 {
				result.Episodes[len(result.Episodes)-1].End = position
			} else if position > 0 {
				prefix, err := ScalarSlice(text, 0, position)
				if err != nil {
					return SplitResult{}, err
				}
				if strings.TrimSpace(prefix) != "" {
					result.Preface = &ScalarSpan{Start: 0, End: position}
				} else {
					headingStart = 0
				}
			}
			result.Episodes = append(result.Episodes, EpisodeBoundary{SeqNo: len(result.Episodes) + 1, Title: title, Start: headingStart})
		}
		position += utf8.RuneCountInString(line) + 1
	}
	count := utf8.RuneCountInString(text)
	if len(result.Episodes) == 0 {
		result.Episodes = append(result.Episodes, EpisodeBoundary{SeqNo: 1, Title: "第 1 集", Start: 0, End: count})
		result.Warnings = append(result.Warnings, "episode_heading_not_found")
	} else {
		result.Episodes[len(result.Episodes)-1].End = count
	}
	if err := ValidateBoundaries(result.Episodes, result.Preface, count); err != nil {
		return SplitResult{}, err
	}
	return result, nil
}

// ValidateBoundaries rejects gaps, overlaps, zero-width episodes and invalid titles.
func ValidateBoundaries(episodes []EpisodeBoundary, preface *ScalarSpan, count int) error {
	if count < 1 || count > MaxScalarCount || len(episodes) < 1 || len(episodes) > MaxChapterImport {
		return ErrInvalidSpan
	}
	next := 0
	if preface != nil {
		if preface.Start != 0 || preface.End <= 0 || preface.End >= count {
			return ErrInvalidSpan
		}
		next = preface.End
	}
	for i, episode := range episodes {
		if episode.SeqNo != i+1 || episode.Start != next || episode.End <= episode.Start || episode.End > count || strings.TrimSpace(episode.Title) == "" || !utf8.ValidString(episode.Title) || strings.ContainsRune(episode.Title, 0) || utf8.RuneCountInString(episode.Title) > MaxSourceTitleScalars || episode.SourceLineageID != nil && *episode.SourceLineageID == uuid.Nil {
			return ErrInvalidSpan
		}
		next = episode.End
	}
	if next != count {
		return ErrInvalidSpan
	}
	return nil
}
