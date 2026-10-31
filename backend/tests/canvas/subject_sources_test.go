package canvas_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type canvasSubjectSources struct {
	source     promptdomain.ReferenceSource
	actor      identity.Principal
	project    uuid.UUID
	found      bool
	err        error
	bindingErr error
	calls      int
}

func (s *canvasSubjectSources) ResolveSubject(_ context.Context, a identity.Principal, p uuid.UUID, _, _ string) (promptdomain.ReferenceSource, bool, error) {
	if a.ID != s.actor.ID || a.ScopeID != s.actor.ScopeID || p != s.project {
		return promptdomain.ReferenceSource{}, false, promptdomain.ErrReferenceUnavailable
	}
	return s.source, s.found, s.err
}
func (s *canvasSubjectSources) ValidateBinding(context.Context, identity.Principal, uuid.UUID, promptdomain.SourceMedia, bool) error {
	s.calls++
	return s.bindingErr
}
func TestSubjectNodeSourceProofRechecked(t *testing.T) {
	id, version := uuid.New(), uuid.New()
	node := domain.Node{NodeType: "subject", Config: domain.NodeConfig{Subject: &domain.SubjectConfig{Version: 1, SubjectID: &id, VersionID: &version}}}
	actor := identity.Principal{ID: uuid.New(), ScopeID: uuid.New()}
	project := uuid.New()
	source := promptdomain.ReferenceSource{Kind: "subject", SourceID: id.String(), SourceVersion: version.String(), Media: []promptdomain.SourceMedia{{BindingID: uuid.NewString()}, {BindingID: uuid.NewString()}, {BindingID: uuid.NewString()}}}
	owner := &canvasSubjectSources{source: source, actor: actor, project: project, found: true}
	check := func() error {
		return application.ValidateSubjectNodes(t.Context(), actor, project, []domain.Node{node}, owner)
	}
	for range 2 {
		if err := check(); err != nil {
			t.Fatal(err)
		}
	}
	if owner.calls != 6 {
		t.Fatal("reused pin skipped media checks", owner.calls)
	}
	for _, mutation := range []string{"unknown", "deleted", "binding_revoked", "version", "scope", "project", "legacy_binding", "empty_media"} {
		t.Run(mutation, func(t *testing.T) {
			changed := *owner
			changed.source = source
			changed.source.Media = append([]promptdomain.SourceMedia(nil), source.Media...)
			switch mutation {
			case "unknown":
				changed.found = false
			case "deleted":
				changed.err = errors.New("deleted")
			case "binding_revoked":
				changed.bindingErr = errors.New("revoked")
			case "version":
				changed.source.SourceVersion = uuid.NewString()
			case "scope":
				changed.actor.ScopeID = uuid.New()
			case "project":
				changed.project = uuid.New()
			case "legacy_binding":
				changed.source.Media[0].BindingID = ""
			case "empty_media":
				changed.source.Media = nil
			}
			if err := application.ValidateSubjectNodes(t.Context(), actor, project, []domain.Node{node}, &changed); !errors.Is(err, promptdomain.ErrReferenceUnavailable) {
				t.Fatal("unproven subject accepted", err)
			}
		})
	}
	if err := application.ValidateSubjectNodes(t.Context(), actor, project, []domain.Node{node}, nil); !errors.Is(err, promptdomain.ErrReferenceUnavailable) {
		t.Fatal("missing owner", err)
	}
	node.Config.Subject = &domain.SubjectConfig{Version: 1}
	if err := application.ValidateSubjectNodes(t.Context(), actor, project, []domain.Node{node}, nil); err != nil {
		t.Fatal("empty node required media", err)
	}
}
