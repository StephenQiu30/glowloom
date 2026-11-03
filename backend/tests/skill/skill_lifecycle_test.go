package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
	"github.com/google/uuid"
)

func TestSkillPackageValidationAndHash(t *testing.T) {
	t.Run("ValidSkillPackage", func(t *testing.T) {
		resources := map[string]string{
			"references/camera_angles.md": "特写、中景、远景拍摄标准指南",
			"prompts/cinematic.txt":       "8k resolution, cinematic lighting",
		}
		pkg, err := domain.NewSkillPackage(
			"storyboard_master",
			"1.0.0",
			"专业分镜设计与机位语言 Skill",
			"# Storyboard Master\n提供分镜编排专业指导",
			resources,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pkg.Hash == "" {
			t.Fatalf("expected non-empty package hash")
		}
		if pkg.Name != "storyboard_master" {
			t.Errorf("unexpected name: %s", pkg.Name)
		}
	})

	t.Run("PathTraversalBlocked", func(t *testing.T) {
		dangerousResources := map[string]string{
			"../../../../etc/passwd": "malicious content",
		}
		_, err := domain.NewSkillPackage(
			"bad_skill",
			"1.0.0",
			"恶意技能",
			"恶意说明",
			dangerousResources,
		)
		if !errors.Is(err, domain.ErrSkillPathTraversal) {
			t.Fatalf("expected ErrSkillPathTraversal, got %v", err)
		}
	})

	t.Run("EmptyContentBlocked", func(t *testing.T) {
		_, err := domain.NewSkillPackage(
			"empty_skill",
			"1.0.0",
			"空技能",
			"   ",
			nil,
		)
		if !errors.Is(err, domain.ErrSkillEmptyContent) {
			t.Fatalf("expected ErrSkillEmptyContent, got %v", err)
		}
	})
}

func TestSkillDynamicLoadingAndReset(t *testing.T) {
	ctx := context.Background()
	reg := application.NewSkillRegistry()

	pkg, err := domain.NewSkillPackage(
		"script_analysis",
		"1.2.0",
		"长文本剧本结构化分析技能",
		"# Script Analysis Skill\n指导剧本人物、场景与对白结构化解析。",
		nil,
	)
	if err != nil {
		t.Fatalf("create skill error: %v", err)
	}

	if err := reg.Register(pkg); err != nil {
		t.Fatalf("register skill: %v", err)
	}

	state := &graph.AgentState{
		RunID: uuid.New(),
	}

	// 1. First invocation: dynamically loads the skill
	args, _ := json.Marshal(application.SkillLoadInput{SkillName: "script_analysis"})
	resJSON, err := reg.SkillLoadToolHandler(ctx, state, args)
	if err != nil {
		t.Fatalf("skill load error: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(resJSON, &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if res["status"] != "loaded" {
		t.Errorf("expected status 'loaded', got %v", res["status"])
	}

	if len(state.LoadedSkills) != 1 || state.LoadedSkills[0] != "script_analysis" {
		t.Errorf("expected script_analysis in LoadedSkills: %v", state.LoadedSkills)
	}

	if len(state.Messages) != 1 {
		t.Fatalf("expected 1 injected message, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, "<skill_instructions name=\"script_analysis\"") {
		t.Errorf("expected skill instructions in message: %s", state.Messages[0].Content)
	}

	// 2. Second invocation: already loaded, idempotent
	resJSON2, err := reg.SkillLoadToolHandler(ctx, state, args)
	if err != nil {
		t.Fatalf("second load error: %v", err)
	}
	var res2 map[string]any
	_ = json.Unmarshal(resJSON2, &res2)
	if res2["status"] != "already_loaded" {
		t.Errorf("expected status 'already_loaded', got %v", res2["status"])
	}

	// 3. Sub-task context reset
	application.ResetSkillScope(state)
	if len(state.LoadedSkills) != 0 {
		t.Errorf("expected LoadedSkills to be empty after reset, got %v", state.LoadedSkills)
	}
}
