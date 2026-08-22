package scripts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	episodePattern = regexp.MustCompile(`(?i)^\s*(?:第\s*)?(\d{1,4})\s*(?:集|话|episode)(?:\s*[-:：.]?\s*(.*))?$`)
	scenePattern   = regexp.MustCompile(`(?i)^\s*(?:场景|scene)\s*[-:：.]?\s*(.+)$`)
	prefixPattern  = regexp.MustCompile(`^\s*(人物|角色|地点|场景|道具|服装|character|characters|location|prop|props|costume|costumes)\s*[-:：.]\s*(.+)$`)
	speakerPattern = regexp.MustCompile(`^\s*([\p{Han}A-Za-z][\p{Han}A-Za-z0-9·_-]{0,24})\s*[:：]\s*(.+)$`)
)

type Anchor struct {
	Line        int `json:"line"`
	StartOffset int `json:"start_offset"`
	EndOffset   int `json:"end_offset"`
}

type NarrativeUnit struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Anchor  Anchor `json:"anchor"`
	Speaker string `json:"speaker,omitempty"`
}

type Scene struct {
	ID         string          `json:"id"`
	Heading    string          `json:"heading"`
	Anchor     Anchor          `json:"anchor"`
	Narratives []NarrativeUnit `json:"narratives"`
}

type Episode struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Anchor        Anchor    `json:"anchor"`
	ContentUnitID uuid.UUID `json:"content_unit_id,omitempty"`
	Scenes        []Scene   `json:"scenes"`
}

type Asset struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	EpisodeNumbers []int    `json:"episode_numbers"`
	Evidence       []Anchor `json:"evidence"`
}

type Analysis struct {
	SourceHash string    `json:"source_hash"`
	Episodes   []Episode `json:"episodes"`
	Characters []Asset   `json:"characters"`
	Locations  []Asset   `json:"locations"`
	Props      []Asset   `json:"props"`
	Costumes   []Asset   `json:"costumes"`
}

type Workspace struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt string    `json:"created_at"`
}

type Project struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	CreatedAt   string    `json:"created_at"`
}

type ScriptRevision struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Name          string    `json:"name"`
	ContentHash   string    `json:"content_hash"`
	ContentLength int       `json:"content_length"`
	Status        string    `json:"status"`
	CreatedAt     string    `json:"created_at"`
}

type Operation struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	ErrorCode string    `json:"error_code,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type Shot struct {
	ID            uuid.UUID  `json:"id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	ContentUnitID uuid.UUID  `json:"content_unit_id"`
	ShotKey       string     `json:"shot_key"`
	Ordinal       int        `json:"ordinal"`
	Status        string     `json:"status"`
	SourceBeatID  *uuid.UUID `json:"source_beat_id,omitempty"`
}

type Candidate struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TargetType  string    `json:"target_type"`
	TargetID    uuid.UUID `json:"target_id"`
	ArtifactID  uuid.UUID `json:"artifact_id"`
	Status      string    `json:"status"`
	Fixture     bool      `json:"fixture"`
	ObjectKey   string    `json:"object_key,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
}

type Selection struct {
	ID               uuid.UUID `json:"id"`
	ProjectID        uuid.UUID `json:"project_id"`
	TargetType       string    `json:"target_type"`
	TargetID         uuid.UUID `json:"target_id"`
	SelectionPurpose string    `json:"selection_purpose"`
	CandidateID      uuid.UUID `json:"candidate_id"`
	Status           string    `json:"status"`
}

func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func ValidateSource(content string) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("script content must not be empty")
	}
	if utf8.RuneCountInString(content) > 2_000_000 {
		return errors.New("script content exceeds the 2,000,000 rune limit")
	}
	return nil
}

func AnalyzeScript(content string) (Analysis, error) {
	if err := ValidateSource(content); err != nil {
		return Analysis{}, err
	}

	lines := strings.SplitAfter(content, "\n")
	analysis := Analysis{SourceHash: HashContent(content)}
	assetIndex := map[string]*Asset{}
	currentEpisode := -1
	currentScene := -1
	offset := 0

	ensureEpisode := func(number int, title string, anchor Anchor) int {
		for i := range analysis.Episodes {
			if analysis.Episodes[i].Number == number {
				if strings.TrimSpace(title) != "" && analysis.Episodes[i].Title == "未命名剧集" {
					analysis.Episodes[i].Title = strings.TrimSpace(title)
				}
				return i
			}
		}
		analysis.Episodes = append(analysis.Episodes, Episode{Number: number, Title: firstNonEmpty(strings.TrimSpace(title), fmt.Sprintf("第%d集", number)), Anchor: anchor})
		return len(analysis.Episodes) - 1
	}
	ensureScene := func(episodeIndex int, heading string, anchor Anchor) int {
		scenes := &analysis.Episodes[episodeIndex].Scenes
		if len(*scenes) > 0 && strings.TrimSpace(heading) == "" {
			return len(*scenes) - 1
		}
		id := fmt.Sprintf("ep-%d-scene-%d", analysis.Episodes[episodeIndex].Number, len(*scenes)+1)
		*scenes = append(*scenes, Scene{ID: id, Heading: firstNonEmpty(strings.TrimSpace(heading), "未命名场景"), Anchor: anchor})
		return len(*scenes) - 1
	}
	addAsset := func(kind, name string, episodeNumber int, anchor Anchor) {
		name = strings.TrimSpace(strings.Trim(name, "，。；;、, "))
		if name == "" || len([]rune(name)) > 80 {
			return
		}
		key := kind + "\x00" + strings.ToLower(name)
		asset, ok := assetIndex[key]
		if !ok {
			asset = &Asset{Kind: kind, Name: name}
			assetIndex[key] = asset
			switch kind {
			case "character":
				analysis.Characters = append(analysis.Characters, *asset)
			case "location":
				analysis.Locations = append(analysis.Locations, *asset)
			case "prop":
				analysis.Props = append(analysis.Props, *asset)
			case "costume":
				analysis.Costumes = append(analysis.Costumes, *asset)
			}
			asset = assetIndex[key]
		}
		if !containsInt(asset.EpisodeNumbers, episodeNumber) {
			asset.EpisodeNumbers = append(asset.EpisodeNumbers, episodeNumber)
			sort.Ints(asset.EpisodeNumbers)
		}
		if !containsAnchor(asset.Evidence, anchor) {
			asset.Evidence = append(asset.Evidence, anchor)
		}
		// The slices above contain value copies; update the canonical output entry.
		updateAsset(&analysis, asset)
	}

	for lineNumber, raw := range lines {
		line := strings.TrimRight(raw, "\r\n")
		trimmed := strings.TrimSpace(line)
		lineStart := offset
		offset += len(raw)
		if trimmed == "" {
			continue
		}
		anchor := Anchor{Line: lineNumber + 1, StartOffset: lineStart, EndOffset: lineStart + len(line)}

		if match := episodePattern.FindStringSubmatch(trimmed); match != nil {
			number, _ := strconv.Atoi(match[1])
			currentEpisode = ensureEpisode(number, match[2], anchor)
			currentScene = -1
			continue
		}
		if currentEpisode < 0 {
			currentEpisode = ensureEpisode(1, "未命名剧集", anchor)
		}
		if match := scenePattern.FindStringSubmatch(trimmed); match != nil {
			currentScene = ensureScene(currentEpisode, match[1], anchor)
			addAsset("location", match[1], analysis.Episodes[currentEpisode].Number, anchor)
			continue
		}
		if currentScene < 0 {
			currentScene = ensureScene(currentEpisode, "未命名场景", anchor)
		}

		if match := prefixPattern.FindStringSubmatch(trimmed); match != nil {
			kind := prefixKind(strings.ToLower(match[1]))
			for _, name := range splitNames(match[2]) {
				addAsset(kind, name, analysis.Episodes[currentEpisode].Number, anchor)
			}
			continue
		}

		kind := "action"
		speaker := ""
		if match := speakerPattern.FindStringSubmatch(trimmed); match != nil && !isMetadataPrefix(match[1]) {
			kind = "dialogue"
			speaker = strings.TrimSpace(match[1])
			addAsset("character", speaker, analysis.Episodes[currentEpisode].Number, anchor)
		}
		unit := NarrativeUnit{ID: fmt.Sprintf("%s-unit-%d", analysis.Episodes[currentEpisode].Scenes[currentScene].ID, len(analysis.Episodes[currentEpisode].Scenes[currentScene].Narratives)+1), Kind: kind, Text: trimmed, Anchor: anchor, Speaker: speaker}
		analysis.Episodes[currentEpisode].Scenes[currentScene].Narratives = append(analysis.Episodes[currentEpisode].Scenes[currentScene].Narratives, unit)
		for _, asset := range allAssets(&analysis) {
			if strings.Contains(trimmed, asset.Name) {
				addAsset(asset.Kind, asset.Name, analysis.Episodes[currentEpisode].Number, anchor)
			}
		}
	}

	if len(analysis.Episodes) == 0 {
		return Analysis{}, errors.New("script contains no non-empty content")
	}
	sort.Slice(analysis.Episodes, func(i, j int) bool { return analysis.Episodes[i].Number < analysis.Episodes[j].Number })
	for i := range analysis.Episodes {
		if len(analysis.Episodes[i].Scenes) == 0 {
			analysis.Episodes[i].Scenes = append(analysis.Episodes[i].Scenes, Scene{ID: fmt.Sprintf("ep-%d-scene-1", analysis.Episodes[i].Number), Heading: "未命名场景", Anchor: analysis.Episodes[i].Anchor})
		}
	}
	return analysis, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAnchor(values []Anchor, target Anchor) bool {
	for _, value := range values {
		if value.Line == target.Line && value.StartOffset == target.StartOffset {
			return true
		}
	}
	return false
}

func splitNames(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return strings.ContainsRune("、,，;；/| ", r) })
}

func prefixKind(prefix string) string {
	switch prefix {
	case "人物", "角色", "character", "characters":
		return "character"
	case "地点", "场景", "location":
		return "location"
	case "道具", "prop", "props":
		return "prop"
	default:
		return "costume"
	}
}

func isMetadataPrefix(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "人物", "角色", "地点", "场景", "道具", "服装", "character", "characters", "location", "prop", "props", "costume", "costumes":
		return true
	default:
		return false
	}
}

func allAssets(analysis *Analysis) []Asset {
	result := make([]Asset, 0, len(analysis.Characters)+len(analysis.Locations)+len(analysis.Props)+len(analysis.Costumes))
	result = append(result, analysis.Characters...)
	result = append(result, analysis.Locations...)
	result = append(result, analysis.Props...)
	result = append(result, analysis.Costumes...)
	return result
}

func updateAsset(analysis *Analysis, asset *Asset) {
	groups := []*[]Asset{&analysis.Characters, &analysis.Locations, &analysis.Props, &analysis.Costumes}
	for _, group := range groups {
		for index := range *group {
			if (*group)[index].Kind == asset.Kind && strings.EqualFold((*group)[index].Name, asset.Name) {
				(*group)[index] = *asset
				return
			}
		}
	}
}
