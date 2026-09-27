package workspace_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

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

func TestProjectLifecycleArchiveAndUnarchive(t *testing.T) {
	project := validProject()
	archivedAt := time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)
	if err := project.Archive(archivedAt, false); err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if project.Status != "archived" || project.ArchivedAt == nil || !project.ArchivedAt.Equal(archivedAt) || project.Revision != 2 {
		t.Fatalf("archive did not persist its lifecycle state: %+v", project)
	}
	if err := project.CanWrite(); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("archived project must be read only, got %v", err)
	}
	if err := project.Archive(archivedAt, false); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("repeated archive must conflict, got %v", err)
	}
	if err := project.Unarchive(); err != nil {
		t.Fatalf("unarchive project: %v", err)
	}
	if project.Status != "active" || project.ArchivedAt != nil || project.Revision != 3 {
		t.Fatalf("unarchive did not restore active state: %+v", project)
	}
	if err := project.CanWrite(); err != nil {
		t.Fatalf("active project should allow writes: %v", err)
	}
	if err := project.Unarchive(); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("repeated unarchive must conflict, got %v", err)
	}
}

func TestProjectLifecycleBlockingOperations(t *testing.T) {
	now := time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)
	project := validProject()
	if err := project.Archive(now, true); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("inflight operations must block archive, got %v", err)
	}
	if err := project.Delete(now, true); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("inflight operations must block deletion, got %v", err)
	}
	if project.Status != "active" || project.IsDelete || project.Revision != 1 {
		t.Fatalf("blocked operations changed the project: %+v", project)
	}
	if err := project.Archive(now, false); err != nil {
		t.Fatalf("archive after operations settle: %v", err)
	}
	if err := project.Delete(now, true); !errors.Is(err, domain.ErrProjectHasInflightOperations) {
		t.Fatalf("inflight operations must also block deletion of archived projects, got %v", err)
	}
}

func TestProjectLifecycleDeleteAndRestoreOriginalState(t *testing.T) {
	deletedAt := time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)
	for _, initialStatus := range []string{"active", "archived"} {
		t.Run(initialStatus, func(t *testing.T) {
			project := validProject()
			if initialStatus == "archived" {
				if err := project.Archive(deletedAt.Add(-time.Hour), false); err != nil {
					t.Fatalf("archive before delete: %v", err)
				}
			}
			beforeDeleteRevision := project.Revision
			archivedAt := project.ArchivedAt
			if err := project.Delete(deletedAt, false); err != nil {
				t.Fatalf("delete project: %v", err)
			}
			if !project.IsDelete || project.Status != initialStatus || project.DeleteTime == nil ||
				!project.DeleteTime.Equal(deletedAt) || project.PurgeAfter == nil ||
				!project.PurgeAfter.Equal(deletedAt.Add(30*24*time.Hour)) || project.Revision != beforeDeleteRevision+1 ||
				project.ArchivedAt != archivedAt {
				t.Fatalf("soft delete did not preserve original status and deadline: %+v", project)
			}
			if err := project.CanWrite(); !errors.Is(err, domain.ErrProjectStateConflict) {
				t.Fatalf("deleted project must reject writes, got %v", err)
			}
			if err := project.Delete(deletedAt, false); !errors.Is(err, domain.ErrProjectStateConflict) {
				t.Fatalf("repeated delete must conflict, got %v", err)
			}
			if err := project.Archive(deletedAt, false); !errors.Is(err, domain.ErrProjectStateConflict) {
				t.Fatalf("deleted project cannot be archived, got %v", err)
			}
			if err := project.Unarchive(); !errors.Is(err, domain.ErrProjectStateConflict) {
				t.Fatalf("deleted project cannot be unarchived, got %v", err)
			}
			if err := project.Restore(deletedAt.Add(10 * 24 * time.Hour)); err != nil {
				t.Fatalf("restore during thirty-day window: %v", err)
			}
			if project.IsDelete || project.Status != initialStatus || project.DeleteTime != nil ||
				project.PurgeAfter != nil || project.Revision != beforeDeleteRevision+2 || project.ArchivedAt != archivedAt {
				t.Fatalf("restore did not recover original state: %+v", project)
			}
			if initialStatus == "archived" {
				if err := project.CanWrite(); !errors.Is(err, domain.ErrProjectStateConflict) {
					t.Fatalf("restored archived project must remain read only, got %v", err)
				}
			} else if err := project.CanWrite(); err != nil {
				t.Fatalf("restored active project should allow writes: %v", err)
			}
		})
	}
}

func TestProjectLifecycleRestoreWindow(t *testing.T) {
	deletedAt := time.Date(2026, time.September, 27, 10, 0, 0, 0, time.UTC)
	beforeDeadline := validProject()
	if err := beforeDeadline.Delete(deletedAt, false); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if err := beforeDeadline.Restore(deletedAt.Add(30*24*time.Hour - time.Nanosecond)); err != nil {
		t.Fatalf("restore before deadline: %v", err)
	}
	for _, elapsed := range []time.Duration{30 * 24 * time.Hour, 31 * 24 * time.Hour} {
		project := validProject()
		if err := project.Delete(deletedAt, false); err != nil {
			t.Fatalf("delete project: %v", err)
		}
		if err := project.Restore(deletedAt.Add(elapsed)); !errors.Is(err, domain.ErrProjectRestoreExpired) {
			t.Fatalf("restore at or after deadline must expire, got %v", err)
		}
		if !project.IsDelete || project.Revision != 2 {
			t.Fatalf("expired restoration changed the project: %+v", project)
		}
	}
	project := validProject()
	if err := project.Restore(deletedAt); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatalf("restore without deletion must conflict, got %v", err)
	}
}

func TestProjectLifecycleRejectsInvalidTimestampAndRevisionOverflow(t *testing.T) {
	project := validProject()
	if err := project.Archive(time.Time{}, false); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("archive with zero timestamp must fail, got %v", err)
	}
	if err := project.Delete(time.Time{}, false); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("delete with zero timestamp must fail, got %v", err)
	}
	project.Revision = math.MaxInt32
	if err := project.Archive(time.Now(), false); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("revision beyond database range must fail, got %v", err)
	}
	if project.Status != "active" || project.Revision != math.MaxInt32 {
		t.Fatalf("invalid transition changed project: %+v", project)
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
