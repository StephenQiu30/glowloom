package workspace_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func validProject() domain.Project {
	return domain.Project{
		ID:          uuid.New(),
		OrgID:       uuid.New(),
		Name:        "逆光",
		AspectRatio: "9:16",
		StyleType:   "realistic",
		Resolution:  "1080p",
		Status:      "active",
		Revision:    1,
	}
}

func TestProjectValidateAcceptsSupportedSpecifications(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.Project)
	}{
		{name: "realistic with overseas models disabled by default"},
		{name: "fifty Chinese characters", change: func(project *domain.Project) {
			project.Name = strings.Repeat("剧", 50)
		}},
		{name: "landscape and archived", change: func(project *domain.Project) {
			project.AspectRatio = "16:9"
			project.Status = "archived"
		}},
		{name: "overseas models allowed explicitly", change: func(project *domain.Project) {
			project.AllowOverseasModels = true
		}},
	}
	for _, subtype := range []string{"anime_jp", "guofeng_xianxia", "cartoon_3d", "manhwa"} {
		tests = append(tests, struct {
			name   string
			change func(*domain.Project)
		}{name: subtype, change: func(project *domain.Project) {
			project.StyleType = "stylized"
			project.StyleSubtype = subtype
		}})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := validProject()
			if test.change != nil {
				test.change(&project)
			}
			if err := project.Validate(); err != nil {
				t.Fatalf("valid project rejected: %v", err)
			}
			if test.name == "realistic with overseas models disabled by default" && project.AllowOverseasModels {
				t.Fatal("new project allows overseas models by default")
			}
		})
	}
}

func TestProjectValidateRejectsInvalidSpecifications(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.Project)
	}{
		{name: "missing ID", change: func(project *domain.Project) { project.ID = uuid.Nil }},
		{name: "missing organization", change: func(project *domain.Project) { project.OrgID = uuid.Nil }},
		{name: "empty name", change: func(project *domain.Project) { project.Name = "" }},
		{name: "blank name", change: func(project *domain.Project) { project.Name = " \t\n " }},
		{name: "fifty-one Chinese characters", change: func(project *domain.Project) {
			project.Name = strings.Repeat("剧", 51)
		}},
		{name: "unsupported aspect ratio", change: func(project *domain.Project) { project.AspectRatio = "4:3" }},
		{name: "missing aspect ratio", change: func(project *domain.Project) { project.AspectRatio = "" }},
		{name: "unknown style type", change: func(project *domain.Project) { project.StyleType = "photorealistic" }},
		{name: "realistic with subtype", change: func(project *domain.Project) { project.StyleSubtype = "anime_jp" }},
		{name: "stylized without subtype", change: func(project *domain.Project) { project.StyleType = "stylized" }},
		{name: "stylized with unknown subtype", change: func(project *domain.Project) {
			project.StyleType = "stylized"
			project.StyleSubtype = "watercolor"
		}},
		{name: "unsupported resolution", change: func(project *domain.Project) { project.Resolution = "720p" }},
		{name: "missing resolution", change: func(project *domain.Project) { project.Resolution = "" }},
		{name: "unknown status", change: func(project *domain.Project) { project.Status = "deleted" }},
		{name: "missing status", change: func(project *domain.Project) { project.Status = "" }},
		{name: "zero revision", change: func(project *domain.Project) { project.Revision = 0 }},
		{name: "negative revision", change: func(project *domain.Project) { project.Revision = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := validProject()
			test.change(&project)
			if err := project.Validate(); err == nil {
				t.Fatal("invalid project accepted")
			}
		})
	}
}
