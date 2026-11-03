package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
	"github.com/google/uuid"
)

// SkillRegistry manages the inventory of registered skill packages.
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*domain.SkillPackage
}

// NewSkillRegistry creates a skill package repository.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]*domain.SkillPackage),
	}
}

// Register stores a validated skill package in the registry.
func (r *SkillRegistry) Register(pkg *domain.SkillPackage) error {
	if pkg == nil {
		return fmt.Errorf("skill package cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[pkg.Name] = pkg
	return nil
}

// Find retrieves a skill package by its unique name.
func (r *SkillRegistry) Find(name string) (*domain.SkillPackage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pkg, ok := r.skills[name]
	if !ok {
		return nil, domain.ErrSkillNotFound
	}
	return pkg, nil
}

// SkillLoadInput is the argument schema for the skill_load tool.
type SkillLoadInput struct {
	SkillName string `json:"skill_name"`
}

// SkillLoadToolHandler implements the skill_load tool to dynamically expand SKILL.md into AgentState.
func (r *SkillRegistry) SkillLoadToolHandler(ctx context.Context, state *graph.AgentState, args json.RawMessage) (json.RawMessage, error) {
	var input SkillLoadInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid skill_load arguments: %w", err)
	}

	pkg, err := r.Find(input.SkillName)
	if err != nil {
		return nil, err
	}

	// Check if already loaded
	for _, loaded := range state.LoadedSkills {
		if loaded == pkg.Name {
			return json.RawMessage(fmt.Sprintf(`{"status":"already_loaded","skill":%q}`, pkg.Name)), nil
		}
	}

	// Record skill pin in state
	state.LoadedSkills = append(state.LoadedSkills, pkg.Name)

	// Inject instruction message into conversation context
	instructionMsg := fmt.Sprintf("<skill_instructions name=%q version=%q hash=%q>\n%s\n</skill_instructions>",
		pkg.Name, pkg.Version, pkg.Hash, pkg.SkillMD)

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "system",
		Content:   instructionMsg,
		Timestamp: time.Now().UTC(),
	})

	resp := map[string]any{
		"status":      "loaded",
		"skill_name":  pkg.Name,
		"version":     pkg.Version,
		"description": pkg.Description,
	}
	rawResp, _ := json.Marshal(resp)
	return rawResp, nil
}

// ResetSkillScope resets the expanded skill context from state for sub-task isolation.
func ResetSkillScope(state *graph.AgentState) {
	if state == nil {
		return
	}
	state.LoadedSkills = nil
}
