package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const ReferenceTargetDependencyDAGSchema = "reference-target-dependency-dag-production"

type ReferenceTargetDependencyNode struct {
	TargetVersionID             string   `json:"target_version_id"`
	TargetBusinessKey           string   `json:"target_business_key"`
	TargetKind                  string   `json:"target_kind"`
	Fulfillment                 string   `json:"fulfillment"`
	TargetContentHash           string   `json:"target_content_hash"`
	WaveKey                     string   `json:"wave_key"`
	DependsOnTargetBusinessKeys []string `json:"depends_on_target_business_keys"`
}

type ReferenceTargetDependencyWave struct {
	WaveKey            string   `json:"wave_key"`
	TargetBusinessKeys []string `json:"target_business_keys"`
}

type ReferenceTargetDependencyDAG struct {
	SchemaVersion string                          `json:"schema_version"`
	PlanVersionID string                          `json:"plan_version_id"`
	Nodes         []ReferenceTargetDependencyNode `json:"nodes"`
	Waves         []ReferenceTargetDependencyWave `json:"waves"`
	ContentHash   string                          `json:"content_hash"`
}

func BuildReferenceTargetDependencyDAG(
	planVersionID string,
	targets []ReferencePlanTargetVersion,
) (ReferenceTargetDependencyDAG, error) {
	if parsed, err := uuid.Parse(planVersionID); err != nil || parsed == uuid.Nil || len(targets) == 0 {
		return ReferenceTargetDependencyDAG{}, errors.New("invalid Reference Target dependency DAG input")
	}
	byKey := make(map[string]ReferencePlanTargetVersion, len(targets))
	versionIDs := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if err := validateDependencyTarget(planVersionID, target); err != nil {
			return ReferenceTargetDependencyDAG{}, err
		}
		if _, duplicate := byKey[target.TargetBusinessKey]; duplicate {
			return ReferenceTargetDependencyDAG{}, errors.New("duplicate Reference Target dependency key")
		}
		if _, duplicate := versionIDs[target.ID]; duplicate {
			return ReferenceTargetDependencyDAG{}, errors.New("duplicate Reference Target Version identity")
		}
		byKey[target.TargetBusinessKey] = target
		versionIDs[target.ID] = struct{}{}
	}
	nodes := make([]ReferenceTargetDependencyNode, 0, len(targets))
	waves := map[string][]string{
		"base": {}, "appearance": {}, "composition": {},
	}
	for _, target := range targets {
		if target.Fulfillment == "not_generated" {
			continue
		}
		waveKey, waveRank := referenceTargetWave(target.TargetKind)
		if err := validateExecutableTargetDependencies(target, waveRank, byKey); err != nil {
			return ReferenceTargetDependencyDAG{}, err
		}
		nodes = append(nodes, ReferenceTargetDependencyNode{
			TargetVersionID: target.ID, TargetBusinessKey: target.TargetBusinessKey,
			TargetKind: target.TargetKind, Fulfillment: target.Fulfillment,
			TargetContentHash: target.ContentHash, WaveKey: waveKey,
			DependsOnTargetBusinessKeys: append([]string{}, target.DependsOnTargetBusinessKeys...),
		})
		waves[waveKey] = append(waves[waveKey], target.TargetBusinessKey)
	}
	if len(nodes) == 0 {
		return ReferenceTargetDependencyDAG{}, errors.New("Reference Target dependency DAG has no executable Target")
	}
	sort.Slice(nodes, func(left, right int) bool {
		return nodes[left].TargetBusinessKey < nodes[right].TargetBusinessKey
	})
	result := ReferenceTargetDependencyDAG{
		SchemaVersion: ReferenceTargetDependencyDAGSchema, PlanVersionID: planVersionID,
		Nodes: nodes,
		Waves: []ReferenceTargetDependencyWave{
			{WaveKey: "base", TargetBusinessKeys: sortedDependencyKeys(waves["base"])},
			{WaveKey: "appearance", TargetBusinessKeys: sortedDependencyKeys(waves["appearance"])},
			{WaveKey: "composition", TargetBusinessKeys: sortedDependencyKeys(waves["composition"])},
		},
	}
	if err := validateReferenceTargetAcyclic(result.Nodes); err != nil {
		return ReferenceTargetDependencyDAG{}, err
	}
	var err error
	result.ContentHash, err = referenceTargetDependencyHash(result)
	if err != nil {
		return ReferenceTargetDependencyDAG{}, err
	}
	return result, nil
}

func validateDependencyTarget(planVersionID string, target ReferencePlanTargetVersion) error {
	if parsed, err := uuid.Parse(target.ID); err != nil || parsed == uuid.Nil ||
		target.PlanVersionID != planVersionID || strings.TrimSpace(target.TargetBusinessKey) == "" ||
		!slices.Contains(referencePlanTargetKinds, target.TargetKind) ||
		!slices.Contains([]string{"not_generated", "optional", "required"}, target.Fulfillment) ||
		!referencePlanHashPattern.MatchString(target.ContentHash) ||
		!sortedStrings(target.DependsOnTargetBusinessKeys, false) {
		return errors.New("invalid Reference Target dependency fact")
	}
	canonicalKey, err := platformcanonical.JSON([]byte(target.TargetBusinessKey))
	if err != nil || string(canonicalKey) != target.TargetBusinessKey {
		return errors.New("invalid Reference Target dependency business key")
	}
	var parts []json.RawMessage
	if err = platformcanonical.Decode(canonicalKey, &parts); err != nil || len(parts) == 0 {
		return errors.New("invalid Reference Target dependency business key")
	}
	var kind string
	if err = json.Unmarshal(parts[0], &kind); err != nil || kind != target.TargetKind {
		return errors.New("Reference Target dependency kind has drifted")
	}
	return nil
}

func validateExecutableTargetDependencies(
	target ReferencePlanTargetVersion,
	waveRank int,
	byKey map[string]ReferencePlanTargetVersion,
) error {
	switch target.TargetKind {
	case "character_identity_anchor", "location_board", "prop_sheet":
		if len(target.DependsOnTargetBusinessKeys) != 0 {
			return errors.New("Reference Target base dependency is invalid")
		}
	case "character_appearance":
		if len(target.DependsOnTargetBusinessKeys) != 1 {
			return errors.New("Character Appearance must depend on one Identity Anchor")
		}
	case "interaction_composition", "scene_composition":
		if len(target.DependsOnTargetBusinessKeys) == 0 {
			return errors.New("Reference composition Target has no base dependency")
		}
	}
	for _, dependencyKey := range target.DependsOnTargetBusinessKeys {
		dependency, exists := byKey[dependencyKey]
		if !exists {
			return errors.New("Reference Target dependency is outside the approved Plan")
		}
		if dependency.Fulfillment == "not_generated" {
			return errors.New("Reference Target depends on a non-executable Target")
		}
		_, dependencyRank := referenceTargetWave(dependency.TargetKind)
		if dependencyRank >= waveRank || referenceTargetFulfillmentRank(dependency.Fulfillment) < referenceTargetFulfillmentRank(target.Fulfillment) {
			return errors.New("Reference Target dependency order is invalid")
		}
		if target.TargetKind == "character_appearance" && dependency.TargetKind != "character_identity_anchor" {
			return errors.New("Character Appearance dependency is not an Identity Anchor")
		}
	}
	return nil
}

func referenceTargetWave(targetKind string) (string, int) {
	switch targetKind {
	case "character_identity_anchor", "location_board", "prop_sheet":
		return "base", 0
	case "character_appearance":
		return "appearance", 1
	default:
		return "composition", 2
	}
}

func referenceTargetFulfillmentRank(value string) int {
	switch value {
	case "required":
		return 2
	case "optional":
		return 1
	default:
		return 0
	}
}

func validateReferenceTargetAcyclic(nodes []ReferenceTargetDependencyNode) error {
	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		indegree[node.TargetBusinessKey] = len(node.DependsOnTargetBusinessKeys)
		for _, dependency := range node.DependsOnTargetBusinessKeys {
			dependents[dependency] = append(dependents[dependency], node.TargetBusinessKey)
		}
	}
	ready := make([]string, 0, len(nodes))
	for key, count := range indegree {
		if count == 0 {
			ready = append(ready, key)
		}
	}
	visited := 0
	for len(ready) > 0 {
		sort.Strings(ready)
		key := ready[0]
		ready = ready[1:]
		visited++
		for _, dependent := range dependents[key] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if visited != len(nodes) {
		return errors.New("Reference Target dependency graph contains a cycle")
	}
	return nil
}

func sortedDependencyKeys(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func referenceTargetDependencyHash(value ReferenceTargetDependencyDAG) (string, error) {
	value.ContentHash = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}
