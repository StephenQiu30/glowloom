package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const StoryGraphSourceInventoryContractID = "storygraph-external-source-inventory-production"

var (
	storyGraphSourceIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	gitCommitPattern          = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

//go:embed manifests/storygraph-source-inventory.json
var pinnedStoryGraphSourceInventory []byte

type ExternalSkillLicense struct {
	SPDXID         string `json:"spdx_id"`
	SourceURL      string `json:"source_url"`
	SHA256         string `json:"sha256"`
	ByteLength     int64  `json:"byte_length"`
	NoticeRequired bool   `json:"notice_required"`
}

type ExternalSkillSource struct {
	SourceID               string               `json:"source_id"`
	RepositoryURL          string               `json:"repository_url"`
	SourceURL              string               `json:"source_url"`
	SourceCommit           string               `json:"source_commit"`
	SourceVersion          string               `json:"source_version"`
	Author                 string               `json:"author"`
	FetchedAt              string               `json:"fetched_at"`
	SourceFileSHA256       string               `json:"source_file_sha256"`
	SourceByteLength       int64                `json:"source_byte_length"`
	License                ExternalSkillLicense `json:"license"`
	ExpectedCapabilityKeys []string             `json:"expected_capability_keys"`
	IntendedUse            string               `json:"intended_use"`
	AbsorptionMode         string               `json:"absorption_mode"`
	ProductionFileMappings []string             `json:"production_file_mappings"`
	ReviewerID             string               `json:"reviewer_id"`
}

type StoryGraphSourceInventory struct {
	ContractID          string                `json:"contract_id"`
	Sources             []ExternalSkillSource `json:"sources"`
	SourceInventoryHash string                `json:"source_inventory_hash"`
}

func PinnedStoryGraphSourceInventory() (StoryGraphSourceInventory, json.RawMessage, error) {
	return DecodeStoryGraphSourceInventory(pinnedStoryGraphSourceInventory)
}

func DecodeStoryGraphSourceInventory(raw json.RawMessage) (StoryGraphSourceInventory, json.RawMessage, error) {
	var inventory StoryGraphSourceInventory
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return StoryGraphSourceInventory{}, nil, errors.New("invalid StoryGraph Source Inventory")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return StoryGraphSourceInventory{}, nil, errors.New("invalid StoryGraph Source Inventory")
	}
	if err := validateStoryGraphSourceInventory(inventory); err != nil {
		return StoryGraphSourceInventory{}, nil, err
	}
	encoded, err := encodeStoryGraphSourceInventory(inventory)
	if err != nil {
		return StoryGraphSourceInventory{}, nil, err
	}
	return inventory, encoded, nil
}

func validateStoryGraphSourceInventory(inventory StoryGraphSourceInventory) error {
	if inventory.ContractID != StoryGraphSourceInventoryContractID ||
		!hashPattern.MatchString(inventory.SourceInventoryHash) || len(inventory.Sources) == 0 {
		return errors.New("invalid StoryGraph Source Inventory")
	}
	for index, source := range inventory.Sources {
		if index > 0 && inventory.Sources[index-1].SourceID >= source.SourceID {
			return errors.New("StoryGraph Source Inventory is not uniquely sorted")
		}
		if err := validateExternalSkillSource(source); err != nil {
			return err
		}
	}
	hash, err := storyGraphSourceInventoryHash(inventory)
	if err != nil || hash != inventory.SourceInventoryHash {
		return errors.New("StoryGraph Source Inventory hash has drifted")
	}
	return nil
}

func validateExternalSkillSource(source ExternalSkillSource) error {
	if !storyGraphSourceIDPattern.MatchString(source.SourceID) || !gitCommitPattern.MatchString(source.SourceCommit) ||
		source.SourceVersion != source.SourceCommit || strings.TrimSpace(source.Author) == "" ||
		strings.TrimSpace(source.IntendedUse) == "" || source.ReviewerID != "project-owner" ||
		source.SourceByteLength < 1 || !hashPattern.MatchString(source.SourceFileSHA256) {
		return errors.New("invalid external Skill source")
	}
	if _, err := time.Parse(time.RFC3339, source.FetchedAt); err != nil {
		return errors.New("invalid external Skill fetch time")
	}
	for _, candidate := range []string{source.RepositoryURL, source.SourceURL, source.License.SourceURL} {
		parsed, err := url.ParseRequestURI(candidate)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
			parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("invalid external Skill source URL")
		}
	}
	repository, _ := url.ParseRequestURI(source.RepositoryURL)
	repositoryPath := strings.Split(strings.Trim(repository.Path, "/"), "/")
	versionPrefix := strings.TrimSuffix(source.RepositoryURL, "/") + "/blob/" + source.SourceCommit + "/"
	if len(repositoryPath) != 2 || !strings.HasPrefix(source.SourceURL, versionPrefix) ||
		!strings.HasPrefix(source.License.SourceURL, versionPrefix) {
		return errors.New("external Skill source URL is not pinned to its repository commit")
	}
	if (source.License.SPDXID != "Apache-2.0" && source.License.SPDXID != "MIT") ||
		!source.License.NoticeRequired || source.License.ByteLength < 1 ||
		!hashPattern.MatchString(source.License.SHA256) {
		return errors.New("invalid external Skill license")
	}
	if err := validateExternalSkillCapabilities(source.ExpectedCapabilityKeys); err != nil {
		return err
	}
	if source.AbsorptionMode != "reference_only" || len(source.ProductionFileMappings) != 0 {
		return errors.New("external Skill source is not isolated from the production Bundle")
	}
	return nil
}

func validateExternalSkillCapabilities(capabilities []string) error {
	allowed := map[string]struct{}{
		"parse-script-structure": {}, "build-production-bible": {}, "map-scene-continuity": {},
		"resolve-visual-foundation": {}, "design-reference-assets": {}, "review-production": {},
		"direct-storyboard": {},
	}
	if len(capabilities) == 0 || !slices.IsSorted(capabilities) {
		return errors.New("invalid external Skill capability mapping")
	}
	for index, capability := range capabilities {
		if _, exists := allowed[capability]; !exists || index > 0 && capabilities[index-1] == capability {
			return errors.New("invalid external Skill capability mapping")
		}
	}
	return nil
}

func storyGraphSourceInventoryHash(inventory StoryGraphSourceInventory) (string, error) {
	inventory.SourceInventoryHash = ""
	raw, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "source_inventory_hash")
	raw, err = json.Marshal(root)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeStoryGraphSourceInventory(inventory StoryGraphSourceInventory) (json.RawMessage, error) {
	raw, err := json.Marshal(inventory)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
