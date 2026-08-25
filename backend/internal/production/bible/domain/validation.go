package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	keyPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,99}$`)
	statePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,79}$`)
	hashPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func DecodeAndValidateCandidate(raw json.RawMessage, normalizedText string) (Candidate, error) {
	var candidate Candidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return Candidate{}, errors.New("candidate does not match production bible schema")
	}
	if err := validateCandidate(candidate, normalizedText); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func validateCandidate(candidate Candidate, normalizedText string) error {
	if len(candidate.Entities) > 5000 || len(candidate.WorldEntries) > 5000 || len(candidate.ReviewIssues) > 5000 {
		return errors.New("candidate exceeds production bible limits")
	}
	entityKeys := make(map[string]struct{}, len(candidate.Entities))
	for _, entity := range candidate.Entities {
		if !keyPattern.MatchString(entity.EntityKey) || !oneOf(entity.Kind, "character", "location", "prop", "costume", "visual_style", "voice") || strings.TrimSpace(entity.CanonicalName) == "" || normalizeName(entity.CanonicalName) != entity.NormalizedName || entity.StableSpec == nil || len(entity.Evidence) == 0 {
			return errors.New("candidate contains an invalid entity")
		}
		if _, exists := entityKeys[entity.EntityKey]; exists {
			return errors.New("candidate entity keys must be unique")
		}
		entityKeys[entity.EntityKey] = struct{}{}
		if err := validateNumbers(entity.EpisodeNumbers); err != nil {
			return err
		}
		if err := validateEvidence(entity.Evidence, normalizedText); err != nil {
			return err
		}
		stateKeys := map[string]struct{}{}
		for _, state := range entity.States {
			if !statePattern.MatchString(state.StateKey) || strings.TrimSpace(state.Label) == "" || state.StateSpec == nil || len(state.Evidence) == 0 {
				return errors.New("candidate contains an invalid entity state")
			}
			if _, exists := stateKeys[state.StateKey]; exists {
				return errors.New("candidate entity state keys must be unique")
			}
			stateKeys[state.StateKey] = struct{}{}
			if err := validateNumbers(state.EpisodeNumbers); err != nil {
				return err
			}
			if err := validateEvidence(state.Evidence, normalizedText); err != nil {
				return err
			}
		}
	}
	worldKeys := map[string]struct{}{}
	for _, entry := range candidate.WorldEntries {
		if !keyPattern.MatchString(entry.EntryKey) || strings.TrimSpace(entry.Category) == "" || strings.TrimSpace(entry.Title) == "" || (len(entry.Facts) == 0 && len(entry.Rules) == 0) || len(entry.Evidence) == 0 {
			return errors.New("candidate contains an invalid world entry")
		}
		if _, exists := worldKeys[entry.EntryKey]; exists {
			return errors.New("candidate world entry keys must be unique")
		}
		worldKeys[entry.EntryKey] = struct{}{}
		for _, entityKey := range entry.EntityKeys {
			if _, exists := entityKeys[entityKey]; !exists {
				return errors.New("candidate world entry references an unknown entity")
			}
		}
		if err := validateNumbers(entry.EpisodeNumbers); err != nil {
			return err
		}
		if err := validateEvidence(entry.Evidence, normalizedText); err != nil {
			return err
		}
	}
	issueKeys := map[string]struct{}{}
	for _, issue := range candidate.ReviewIssues {
		if !keyPattern.MatchString(issue.IssueKey) || strings.TrimSpace(issue.Code) == "" || !oneOf(issue.Severity, "warning", "blocking") || !oneOf(issue.Scope, "global", "entity", "entity_state", "world_entry") || strings.TrimSpace(issue.Summary) == "" || (issue.Scope == "global") != (issue.SubjectKey == nil) {
			return errors.New("candidate contains an invalid review issue")
		}
		if _, exists := issueKeys[issue.IssueKey]; exists {
			return errors.New("candidate review issue keys must be unique")
		}
		issueKeys[issue.IssueKey] = struct{}{}
		if err := validateEvidence(issue.Evidence, normalizedText); err != nil {
			return err
		}
	}
	return nil
}

func validateEvidence(items []Evidence, normalizedText string) error {
	text := []rune(normalizedText)
	for _, evidence := range items {
		anchor := []rune(evidence.ExactAnchor)
		if evidence.SourceStart < 0 || evidence.SourceEnd <= evidence.SourceStart || evidence.SourceEnd > len(text) || len(anchor) != evidence.SourceEnd-evidence.SourceStart || string(text[evidence.SourceStart:evidence.SourceEnd]) != evidence.ExactAnchor || !hashPattern.MatchString(evidence.TextHash) {
			return errors.New("candidate evidence does not match the immutable source")
		}
		hash := sha256.Sum256([]byte(evidence.ExactAnchor))
		if hex.EncodeToString(hash[:]) != evidence.TextHash || (evidence.EpisodeNumber != nil && *evidence.EpisodeNumber < 1) || !utf8.ValidString(evidence.ExactAnchor) {
			return errors.New("candidate evidence hash is invalid")
		}
	}
	return nil
}

func validateNumbers(values []int) error {
	if !sort.IntsAreSorted(values) {
		return errors.New("candidate episode numbers must be sorted")
	}
	for index, value := range values {
		if value < 1 || (index > 0 && values[index-1] == value) {
			return errors.New("candidate episode numbers must be positive and unique")
		}
	}
	return nil
}

func normalizeName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
