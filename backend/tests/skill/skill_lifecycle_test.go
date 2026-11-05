package skill_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/skill/domain"
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
