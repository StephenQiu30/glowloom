package bible_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	"github.com/google/uuid"
)

type textQueryProject struct {
	value projectdomain.Project
	err   error
}

func (p textQueryProject) Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error) {
	return p.value, p.err
}

type textQueryStore struct {
	value domain.TextWorldVersion
	calls int
}

func (s *textQueryStore) ReadTextWorld(context.Context, string, string, string) (domain.TextWorldVersion, error) {
	s.calls++
	return s.value, nil
}
func TestTextWorldQueryRequiresProjectAccessAndFrozenContent(t *testing.T) {
	input := textWorldFixture(t)
	value, err := app.BuildTextWorld(input)
	if err != nil {
		t.Fatal(err)
	}
	store := &textQueryStore{value: value}
	project := textQueryProject{value: projectdomain.Project{ID: value.ProjectID, WorkspaceID: value.WorkspaceID}}
	service := app.NewTextWorldQuery(store, project)
	got, err := service.Get(context.Background(), app.Actor{}, value.ProjectID, value.ID)
	if err != nil || got.ContentHash != value.ContentHash {
		t.Fatalf("query=%+v err=%v", got, err)
	}
	for _, mode := range []string{"version", "scope", "hash"} {
		t.Run(mode, func(t *testing.T) {
			store.value = value
			switch mode {
			case "version":
				store.value.Revision++
			case "scope":
				store.value.ProjectID = uuid.NewString()
			case "hash":
				store.value.ContentHash = "drift"
			}
			if _, err = service.Get(context.Background(), app.Actor{}, value.ProjectID, value.ID); err == nil {
				t.Fatal("corrupt version returned")
			}
		})
	}
	denied := errors.New("current permission revoked")
	project.err = denied
	store.calls = 0
	service = app.NewTextWorldQuery(store, project)
	if _, err = service.Get(context.Background(), app.Actor{}, value.ProjectID, value.ID); !errors.Is(err, denied) || store.calls != 0 {
		t.Fatal("read before current authorization")
	}
}
