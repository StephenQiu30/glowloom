package scripts

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

var (
	episodePattern = regexp.MustCompile(`(?i)^\s*(?:#{1,6}\s*)?(?:第\s*)?(\d{1,4})\s*(?:集|话|episode)(?:\s*[-:：.]?\s*(.*))?$`)
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
	TemporaryKey  string    `json:"temporary_key"`
	Ordinal       int       `json:"ordinal"`
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Anchor        Anchor    `json:"anchor"`
	BoundaryRule  string    `json:"boundary_rule"`
	Decision      string    `json:"decision"`
	ContentUnitID uuid.UUID `json:"content_unit_id,omitempty"`
	Scenes        []Scene   `json:"scenes"`
}

type BreakdownStatus string

const (
	BreakdownStatusReady   BreakdownStatus = "ready"
	BreakdownStatusBlocked BreakdownStatus = "blocked"
)

type BreakdownIssue struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	CandidateKeys []string `json:"candidate_keys,omitempty"`
	Anchor        *Anchor  `json:"anchor,omitempty"`
}

type EpisodeBreakdown struct {
	RevisionNo       int              `json:"revision_no"`
	Status           BreakdownStatus  `json:"status"`
	CoverageHash     string           `json:"coverage_hash"`
	SegmentationHash string           `json:"segmentation_hash"`
	Issues           []BreakdownIssue `json:"issues"`
}

type BreakdownOperationType string

const (
	BreakdownOperationSplit        BreakdownOperationType = "split"
	BreakdownOperationMerge        BreakdownOperationType = "merge"
	BreakdownOperationMoveBoundary BreakdownOperationType = "move_boundary"
	BreakdownOperationRename       BreakdownOperationType = "rename"
	BreakdownOperationReorder      BreakdownOperationType = "reorder"
	BreakdownOperationIgnore       BreakdownOperationType = "ignore"
)

type EpisodeBreakdownOperation struct {
	Type                 BreakdownOperationType `json:"type"`
	CandidateKey         string                 `json:"candidate_key,omitempty"`
	CandidateKeys        []string               `json:"candidate_keys,omitempty"`
	OrderedCandidateKeys []string               `json:"ordered_candidate_keys,omitempty"`
	BoundaryOffset       int                    `json:"boundary_offset,omitempty"`
	LeftKey              string                 `json:"left_key,omitempty"`
	LeftTitle            string                 `json:"left_title,omitempty"`
	RightKey             string                 `json:"right_key,omitempty"`
	RightTitle           string                 `json:"right_title,omitempty"`
	TargetKey            string                 `json:"target_key,omitempty"`
	TargetTitle          string                 `json:"target_title,omitempty"`
	Title                string                 `json:"title,omitempty"`
}

type Asset struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	EpisodeNumbers []int    `json:"episode_numbers"`
	Evidence       []Anchor `json:"evidence"`
}

type Analysis struct {
	SourceHash  string           `json:"source_hash"`
	ParseReport ParseReport      `json:"parse_report"`
	Breakdown   EpisodeBreakdown `json:"breakdown"`
	Episodes    []Episode        `json:"episodes"`
	Characters  []Asset          `json:"characters"`
	Locations   []Asset          `json:"locations"`
	Props       []Asset          `json:"props"`
	Costumes    []Asset          `json:"costumes"`
}

type ParseReport struct {
	Status         string   `json:"status"`
	Format         string   `json:"format"`
	ParserVersion  string   `json:"parser_version"`
	OriginalHash   string   `json:"original_hash"`
	TextHash       string   `json:"text_hash"`
	CharacterCount int      `json:"character_count"`
	ParagraphCount int      `json:"paragraph_count"`
	FailedScopes   []string `json:"failed_scopes"`
}

type Workspace struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt string    `json:"created_at"`
}

type Project struct {
	ID             uuid.UUID        `json:"id"`
	WorkspaceID    uuid.UUID        `json:"workspace_id"`
	Name           string           `json:"name"`
	CreatedAt      string           `json:"created_at"`
	LatestWorkflow *ProjectWorkflow `json:"latest_workflow,omitempty"`
}

type ProjectWorkflow struct {
	ProjectID        uuid.UUID `json:"project_id"`
	SourceRevisionID uuid.UUID `json:"source_revision_id"`
	SourceStatus     string    `json:"source_status"`
	OperationID      uuid.UUID `json:"operation_id"`
	OperationStatus  string    `json:"operation_status"`
	Progress         int       `json:"progress"`
}

type ProjectPage struct {
	Items    []Project `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

type ProjectQuery struct {
	Page     int
	PageSize int
}

type ScriptRevision struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Name          string    `json:"name"`
	ContentHash   string    `json:"content_hash"`
	ContentLength int       `json:"content_length"`
	SourceType    string    `json:"source_type"`
	Status        string    `json:"status"`
	CreatedAt     string    `json:"created_at"`
}

type Operation struct {
	ID               uuid.UUID  `json:"id"`
	ProjectID        uuid.UUID  `json:"project_id"`
	SourceRevisionID *uuid.UUID `json:"source_revision_id,omitempty"`
	Type             string     `json:"type"`
	Status           string     `json:"status"`
	Progress         int        `json:"progress"`
	ErrorCode        string     `json:"error_code,omitempty"`
	Error            string     `json:"error,omitempty"`
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

type WorkspaceEnvelope struct {
	Data Workspace `json:"data"`
}

type ProjectEnvelope struct {
	Data Project `json:"data"`
}

type ProjectPageEnvelope struct {
	Data ProjectPage `json:"data"`
}

type ScriptRevisionEnvelope struct {
	Data ScriptRevision `json:"data"`
}

type OperationEnvelope struct {
	Data Operation `json:"data"`
}

type AnalysisEnvelope struct {
	Data Analysis `json:"data"`
}

type ShotList struct {
	Items []Shot `json:"items"`
}

type ShotListEnvelope struct {
	Data ShotList `json:"data"`
}

type CandidateEnvelope struct {
	Data Candidate `json:"data"`
}

type SelectionEnvelope struct {
	Data Selection `json:"data"`
}

func HashContent(content string) string {
	return toolkit.SHA256String(content)
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
	analysis := Analysis{SourceHash: HashContent(content), Breakdown: EpisodeBreakdown{RevisionNo: 1}}
	assetIndex := map[string]*Asset{}
	currentEpisode := -1
	currentScene := -1
	offset := 0

	appendEpisode := func(number int, title, rule string, anchor Anchor) int {
		keyMaterial := fmt.Sprintf("%s:%d:%d:%s", analysis.SourceHash, anchor.StartOffset, number, rule)
		analysis.Episodes = append(analysis.Episodes, Episode{
			TemporaryKey: "episode-" + HashContent(keyMaterial)[:16],
			Ordinal:      len(analysis.Episodes) + 1, Number: number,
			Title:  firstNonEmpty(strings.TrimSpace(title), fmt.Sprintf("第%d集", number)),
			Anchor: anchor, BoundaryRule: rule, Decision: "pending", Scenes: []Scene{},
		})
		return len(analysis.Episodes) - 1
	}
	ensureScene := func(episodeIndex int, heading string, anchor Anchor) int {
		scenes := &analysis.Episodes[episodeIndex].Scenes
		if len(*scenes) > 0 && strings.TrimSpace(heading) == "" {
			return len(*scenes) - 1
		}
		id := fmt.Sprintf("%s-scene-%d", analysis.Episodes[episodeIndex].TemporaryKey, len(*scenes)+1)
		*scenes = append(*scenes, Scene{ID: id, Heading: firstNonEmpty(strings.TrimSpace(heading), "未命名场景"), Anchor: anchor})
		return len(*scenes) - 1
	}
	finalizeScene := func(end int) {
		if currentEpisode < 0 || currentScene < 0 {
			return
		}
		scene := &analysis.Episodes[currentEpisode].Scenes[currentScene]
		if end > scene.Anchor.StartOffset {
			scene.Anchor.EndOffset = end
		}
	}
	finalizeEpisode := func(end int) {
		if currentEpisode < 0 {
			return
		}
		finalizeScene(end)
		episode := &analysis.Episodes[currentEpisode]
		if end > episode.Anchor.StartOffset {
			episode.Anchor.EndOffset = end
		}
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
			finalizeEpisode(lineStart)
			number, _ := strconv.Atoi(match[1])
			currentEpisode = appendEpisode(number, match[2], "explicit_episode_heading_v1", anchor)
			currentScene = -1
			continue
		}
		if currentEpisode < 0 {
			currentEpisode = appendEpisode(1, "待校对来源范围", "unlabeled_source_range", anchor)
		}
		if match := scenePattern.FindStringSubmatch(trimmed); match != nil {
			finalizeScene(lineStart)
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
	finalizeEpisode(len(content))

	if len(analysis.Episodes) == 0 {
		return Analysis{}, errors.New("script contains no non-empty content")
	}
	for i := range analysis.Episodes {
		if len(analysis.Episodes[i].Scenes) == 0 {
			analysis.Episodes[i].Scenes = append(analysis.Episodes[i].Scenes, Scene{
				ID: analysis.Episodes[i].TemporaryKey + "-scene-1", Heading: "未命名场景", Anchor: analysis.Episodes[i].Anchor,
			})
		}
	}
	refreshEpisodeBreakdown(&analysis)
	return analysis, nil
}

func ReviseEpisodeBreakdown(current Analysis, expectedSourceHash string, operations []EpisodeBreakdownOperation) (Analysis, error) {
	if expectedSourceHash == "" || expectedSourceHash != current.SourceHash {
		return Analysis{}, errors.New("episode breakdown source basis is stale")
	}
	if len(operations) < 1 || len(operations) > 100 {
		return Analysis{}, errors.New("episode breakdown revision requires 1 to 100 operations")
	}
	revised, err := cloneAnalysis(current)
	if err != nil {
		return Analysis{}, err
	}
	for _, operation := range operations {
		if err := applyEpisodeBreakdownOperation(&revised, operation); err != nil {
			return Analysis{}, err
		}
	}
	for index := range revised.Episodes {
		revised.Episodes[index].Ordinal = index + 1
		revised.Episodes[index].Number = index + 1
		revised.Episodes[index].ContentUnitID = uuid.Nil
	}
	rebuildAssetEpisodeNumbers(&revised)
	revised.Breakdown.RevisionNo = current.Breakdown.RevisionNo + 1
	refreshEpisodeBreakdown(&revised)
	if revised.Breakdown.Status != BreakdownStatusReady {
		return Analysis{}, fmt.Errorf("episode breakdown is blocked: %s", revised.Breakdown.Issues[0].Code)
	}
	return revised, nil
}

func applyEpisodeBreakdownOperation(analysis *Analysis, operation EpisodeBreakdownOperation) error {
	switch operation.Type {
	case BreakdownOperationSplit:
		index := episodeIndexByKey(analysis.Episodes, operation.CandidateKey)
		if index < 0 {
			return errors.New("split candidate does not exist")
		}
		if err := validateCandidateIdentity(operation.LeftKey, operation.LeftTitle); err != nil {
			return err
		}
		if err := validateCandidateIdentity(operation.RightKey, operation.RightTitle); err != nil {
			return err
		}
		if operation.LeftKey == operation.RightKey || episodeKeyExistsExcept(analysis.Episodes, operation.LeftKey, index) || episodeKeyExistsExcept(analysis.Episodes, operation.RightKey, index) {
			return errors.New("split candidate keys must be unique")
		}
		original := analysis.Episodes[index]
		if operation.BoundaryOffset <= original.Anchor.StartOffset || operation.BoundaryOffset >= original.Anchor.EndOffset {
			return errors.New("split boundary must be inside the candidate range")
		}
		leftScenes, rightScenes, err := partitionScenes(original.Scenes, operation.BoundaryOffset)
		if err != nil {
			return err
		}
		if len(leftScenes) == 0 || len(rightScenes) == 0 {
			return errors.New("split must leave at least one complete scene on each side")
		}
		left := original
		left.TemporaryKey, left.Title, left.Scenes = strings.TrimSpace(operation.LeftKey), strings.TrimSpace(operation.LeftTitle), leftScenes
		left.Anchor.EndOffset = operation.BoundaryOffset
		left.BoundaryRule, left.Decision = "manual_split", "accepted_with_changes"
		right := original
		right.TemporaryKey, right.Title, right.Scenes = strings.TrimSpace(operation.RightKey), strings.TrimSpace(operation.RightTitle), rightScenes
		right.Anchor.StartOffset = operation.BoundaryOffset
		right.Anchor.Line = firstSceneLine(rightScenes, original.Anchor.Line)
		right.BoundaryRule, right.Decision = "manual_split", "accepted_with_changes"
		analysis.Episodes = append(analysis.Episodes[:index], append([]Episode{left, right}, analysis.Episodes[index+1:]...)...)
		return nil
	case BreakdownOperationRename:
		index := episodeIndexByKey(analysis.Episodes, operation.CandidateKey)
		if index < 0 {
			return errors.New("rename candidate does not exist")
		}
		title := strings.TrimSpace(operation.Title)
		if len([]rune(title)) < 1 || len([]rune(title)) > 200 {
			return errors.New("episode title must contain 1 to 200 characters")
		}
		analysis.Episodes[index].Title = title
		analysis.Episodes[index].Decision = "accepted_with_changes"
		return nil
	case BreakdownOperationIgnore:
		index := episodeIndexByKey(analysis.Episodes, operation.CandidateKey)
		if index < 0 {
			return errors.New("ignore candidate does not exist")
		}
		reason := strings.TrimSpace(operation.Title)
		if len([]rune(reason)) < 1 || len([]rune(reason)) > 200 {
			return errors.New("ignored range reason must contain 1 to 200 characters")
		}
		analysis.Episodes[index].Title = reason
		analysis.Episodes[index].BoundaryRule = "manual_ignore"
		analysis.Episodes[index].Decision = "ignored"
		return nil
	case BreakdownOperationReorder:
		if len(operation.OrderedCandidateKeys) != len(analysis.Episodes) {
			return errors.New("reorder must contain the complete candidate key set")
		}
		ordered := make([]Episode, 0, len(analysis.Episodes))
		seen := map[string]bool{}
		for _, key := range operation.OrderedCandidateKeys {
			if seen[key] {
				return errors.New("reorder candidate keys must be unique")
			}
			index := episodeIndexByKey(analysis.Episodes, key)
			if index < 0 {
				return errors.New("reorder candidate does not exist")
			}
			seen[key] = true
			ordered = append(ordered, analysis.Episodes[index])
		}
		analysis.Episodes = ordered
		return nil
	case BreakdownOperationMerge:
		if len(operation.CandidateKeys) < 2 {
			return errors.New("merge requires at least two adjacent candidates")
		}
		if err := validateCandidateIdentity(operation.TargetKey, operation.TargetTitle); err != nil {
			return err
		}
		start := episodeIndexByKey(analysis.Episodes, operation.CandidateKeys[0])
		if start < 0 || start+len(operation.CandidateKeys) > len(analysis.Episodes) {
			return errors.New("merge candidates do not exist")
		}
		for offset, key := range operation.CandidateKeys {
			if analysis.Episodes[start+offset].TemporaryKey != key {
				return errors.New("merge candidates must be adjacent and ordered")
			}
		}
		if episodeKeyExistsOutsideRange(analysis.Episodes, operation.TargetKey, start, start+len(operation.CandidateKeys)) {
			return errors.New("merge target key already exists")
		}
		merged := analysis.Episodes[start]
		merged.TemporaryKey, merged.Title = strings.TrimSpace(operation.TargetKey), strings.TrimSpace(operation.TargetTitle)
		merged.BoundaryRule, merged.Decision = "manual_merge", "accepted_with_changes"
		merged.Scenes = nil
		for offset := range operation.CandidateKeys {
			candidate := analysis.Episodes[start+offset]
			if offset > 0 && analysis.Episodes[start+offset-1].Anchor.EndOffset != candidate.Anchor.StartOffset {
				return errors.New("merge candidate ranges must be continuous")
			}
			merged.Scenes = append(merged.Scenes, candidate.Scenes...)
			merged.Anchor.EndOffset = candidate.Anchor.EndOffset
		}
		analysis.Episodes = append(analysis.Episodes[:start], append([]Episode{merged}, analysis.Episodes[start+len(operation.CandidateKeys):]...)...)
		return nil
	case BreakdownOperationMoveBoundary:
		leftIndex := episodeIndexByKey(analysis.Episodes, operation.LeftKey)
		rightIndex := episodeIndexByKey(analysis.Episodes, operation.RightKey)
		if leftIndex < 0 || rightIndex != leftIndex+1 {
			return errors.New("boundary candidates must be adjacent and ordered")
		}
		left, right := analysis.Episodes[leftIndex], analysis.Episodes[rightIndex]
		if operation.BoundaryOffset <= left.Anchor.StartOffset || operation.BoundaryOffset >= right.Anchor.EndOffset {
			return errors.New("boundary must leave both candidates non-empty")
		}
		allScenes := append(append([]Scene{}, left.Scenes...), right.Scenes...)
		leftScenes, rightScenes, err := partitionScenes(allScenes, operation.BoundaryOffset)
		if err != nil {
			return err
		}
		if len(leftScenes) == 0 || len(rightScenes) == 0 {
			return errors.New("boundary must leave at least one complete scene on each side")
		}
		left.Scenes, left.Anchor.EndOffset, left.Decision = leftScenes, operation.BoundaryOffset, "accepted_with_changes"
		right.Scenes, right.Anchor.StartOffset, right.Decision = rightScenes, operation.BoundaryOffset, "accepted_with_changes"
		right.Anchor.Line = firstSceneLine(rightScenes, right.Anchor.Line)
		analysis.Episodes[leftIndex], analysis.Episodes[rightIndex] = left, right
		return nil
	default:
		return fmt.Errorf("unsupported episode breakdown operation %q", operation.Type)
	}
}

func refreshEpisodeBreakdown(analysis *Analysis) {
	issues := make([]BreakdownIssue, 0)
	numbers := map[int][]string{}
	publishedCount := 0
	for _, episode := range analysis.Episodes {
		if episode.Decision != "ignored" {
			publishedCount++
			numbers[episode.Number] = append(numbers[episode.Number], episode.TemporaryKey)
		}
		if episode.Decision != "ignored" && episode.BoundaryRule == "unlabeled_source_range" {
			anchor := episode.Anchor
			issues = append(issues, BreakdownIssue{Code: "unlabeled_episode_range", Message: "来源范围缺少可证明的剧集标题，需要人工校对", CandidateKeys: []string{episode.TemporaryKey}, Anchor: &anchor})
		}
	}
	if publishedCount == 0 {
		issues = append(issues, BreakdownIssue{Code: "all_episode_ranges_ignored", Message: "至少保留一个可发布剧集", CandidateKeys: nil})
	}
	for number, keys := range numbers {
		if len(keys) > 1 {
			issues = append([]BreakdownIssue{{Code: "duplicate_episode_number", Message: fmt.Sprintf("集号 %d 重复，需要人工拆解或重排", number), CandidateKeys: keys}}, issues...)
		}
	}
	bySource := append([]Episode(nil), analysis.Episodes...)
	sort.Slice(bySource, func(i, j int) bool { return bySource[i].Anchor.StartOffset < bySource[j].Anchor.StartOffset })
	for index, episode := range bySource {
		if episode.Anchor.EndOffset <= episode.Anchor.StartOffset {
			issues = append(issues, BreakdownIssue{Code: "empty_episode_range", Message: "剧集来源范围为空", CandidateKeys: []string{episode.TemporaryKey}})
		}
		if index > 0 {
			previous := bySource[index-1]
			switch {
			case previous.Anchor.EndOffset < episode.Anchor.StartOffset:
				issues = append(issues, BreakdownIssue{Code: "episode_coverage_gap", Message: "剧集来源范围之间存在未覆盖内容", CandidateKeys: []string{previous.TemporaryKey, episode.TemporaryKey}})
			case previous.Anchor.EndOffset > episode.Anchor.StartOffset:
				issues = append(issues, BreakdownIssue{Code: "episode_coverage_overlap", Message: "剧集来源范围发生重叠", CandidateKeys: []string{previous.TemporaryKey, episode.TemporaryKey}})
			}
		}
		for _, scene := range episode.Scenes {
			if scene.Anchor.StartOffset < episode.Anchor.StartOffset || scene.Anchor.EndOffset > episode.Anchor.EndOffset {
				issues = append(issues, BreakdownIssue{Code: "scene_crosses_episode", Message: "场次范围越过剧集边界", CandidateKeys: []string{episode.TemporaryKey}})
			}
		}
	}
	analysis.Breakdown.Issues = issues
	analysis.Breakdown.Status = BreakdownStatusReady
	if len(issues) > 0 {
		analysis.Breakdown.Status = BreakdownStatusBlocked
	}
	analysis.Breakdown.CoverageHash = hashBreakdownCoverage(bySource)
	analysis.Breakdown.SegmentationHash = hashBreakdownSegmentation(analysis.Episodes)
}

func cloneAnalysis(value Analysis) (Analysis, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return Analysis{}, fmt.Errorf("encode analysis clone: %w", err)
	}
	var result Analysis
	if err := json.Unmarshal(encoded, &result); err != nil {
		return Analysis{}, fmt.Errorf("decode analysis clone: %w", err)
	}
	return result, nil
}

func partitionScenes(scenes []Scene, boundary int) ([]Scene, []Scene, error) {
	left, right := make([]Scene, 0), make([]Scene, 0)
	for _, scene := range scenes {
		switch {
		case scene.Anchor.EndOffset <= boundary:
			left = append(left, scene)
		case scene.Anchor.StartOffset >= boundary:
			right = append(right, scene)
		default:
			return nil, nil, errors.New("episode boundary cannot split a scene")
		}
	}
	return left, right, nil
}

func validateCandidateIdentity(key, title string) error {
	key = strings.TrimSpace(key)
	title = strings.TrimSpace(title)
	if key == "" || len([]rune(key)) > 120 {
		return errors.New("candidate key must contain 1 to 120 characters")
	}
	if len([]rune(title)) < 1 || len([]rune(title)) > 200 {
		return errors.New("candidate title must contain 1 to 200 characters")
	}
	return nil
}

func episodeIndexByKey(episodes []Episode, key string) int {
	for index := range episodes {
		if episodes[index].TemporaryKey == strings.TrimSpace(key) {
			return index
		}
	}
	return -1
}

func episodeKeyExistsExcept(episodes []Episode, key string, except int) bool {
	index := episodeIndexByKey(episodes, strings.TrimSpace(key))
	return index >= 0 && index != except
}

func episodeKeyExistsOutsideRange(episodes []Episode, key string, start, end int) bool {
	index := episodeIndexByKey(episodes, strings.TrimSpace(key))
	return index >= 0 && (index < start || index >= end)
}

func firstSceneLine(scenes []Scene, fallback int) int {
	if len(scenes) == 0 {
		return fallback
	}
	return scenes[0].Anchor.Line
}

func hashBreakdownCoverage(episodes []Episode) string {
	parts := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		parts = append(parts, fmt.Sprintf("%s:%d:%d", episode.TemporaryKey, episode.Anchor.StartOffset, episode.Anchor.EndOffset))
	}
	return HashContent(strings.Join(parts, "|"))
}

func hashBreakdownSegmentation(episodes []Episode) string {
	parts := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		parts = append(parts, fmt.Sprintf("%d:%s:%s:%d:%d", episode.Ordinal, episode.TemporaryKey, episode.Title, episode.Anchor.StartOffset, episode.Anchor.EndOffset))
	}
	return HashContent(strings.Join(parts, "|"))
}

func rebuildAssetEpisodeNumbers(analysis *Analysis) {
	groups := []*[]Asset{&analysis.Characters, &analysis.Locations, &analysis.Props, &analysis.Costumes}
	for _, group := range groups {
		for index := range *group {
			numbers := make([]int, 0)
			for _, evidence := range (*group)[index].Evidence {
				for _, episode := range analysis.Episodes {
					if episode.Decision != "ignored" && evidence.StartOffset >= episode.Anchor.StartOffset && evidence.StartOffset < episode.Anchor.EndOffset && !containsInt(numbers, episode.Number) {
						numbers = append(numbers, episode.Number)
					}
				}
			}
			sort.Ints(numbers)
			(*group)[index].EpisodeNumbers = numbers
		}
	}
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
