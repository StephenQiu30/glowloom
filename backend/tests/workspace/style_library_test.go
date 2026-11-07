package workspace_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type styleLibraryStub struct {
	calls int
	input app.StyleLibraryMutation
}

func (s *styleLibraryStub) ListStyleLibrary(context.Context, identity.Principal, app.StyleLibraryFilter) (app.StyleLibraryPage, error) {
	s.calls++
	return app.StyleLibraryPage{Items: []app.StyleLibraryEntry{}}, nil
}
func (s *styleLibraryStub) ReadStyleLibrary(context.Context, identity.Principal, uuid.UUID) (app.StyleLibraryDetail, error) {
	s.calls++
	return app.StyleLibraryDetail{}, nil
}
func (s *styleLibraryStub) ChangeStyleLibrary(_ context.Context, _ identity.Principal, in app.StyleLibraryMutation) (app.StyleLibraryChangeResult, error) {
	s.calls++
	s.input = in
	return app.StyleLibraryChangeResult{}, errors.New("fixture stop")
}
func styleLibraryActor() identity.Principal {
	return identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
}
func styleLibraryInput() app.StyleLibraryMutation {
	return app.StyleLibraryMutation{Action: "create", IdempotencyKey: uuid.New(), RequestID: uuid.NewString(), Name: "我的风格", PromptFragment: "保持水墨线条", Category: "2d", ReferenceAssetIDs: []uuid.UUID{}}
}
func TestStyleLibraryRejectsInvalidWritesBeforeStore(t *testing.T) {
	for name, change := range map[string]func(*app.StyleLibraryMutation){
		"blank name":               func(v *app.StyleLibraryMutation) { v.Name = " \n " },
		"21 unicode name":          func(v *app.StyleLibraryMutation) { v.Name = strings.Repeat("风", 21) },
		"blank description":        func(v *app.StyleLibraryMutation) { v.PromptFragment = " \n" },
		"2001 unicode description": func(v *app.StyleLibraryMutation) { v.PromptFragment = strings.Repeat("风", 2001) },
		"category":                 func(v *app.StyleLibraryMutation) { v.Category = "unknown" },
		"five images": func(v *app.StyleLibraryMutation) {
			v.ReferenceAssetIDs = []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		},
		"duplicate image":         func(v *app.StyleLibraryMutation) { id := uuid.New(); v.ReferenceAssetIDs = []uuid.UUID{id, id} },
		"nil image":               func(v *app.StyleLibraryMutation) { v.ReferenceAssetIDs = []uuid.UUID{uuid.Nil} },
		"missing key":             func(v *app.StyleLibraryMutation) { v.IdempotencyKey = uuid.Nil },
		"patch without revision":  func(v *app.StyleLibraryMutation) { v.Action = "patch"; v.ID = uuid.New() },
		"delete without revision": func(v *app.StyleLibraryMutation) { v.Action = "delete"; v.ID = uuid.New() },
	} {
		t.Run(name, func(t *testing.T) {
			s := &styleLibraryStub{}
			in := styleLibraryInput()
			change(&in)
			_, err := app.NewStyleLibrary(s).Change(t.Context(), styleLibraryActor(), in)
			if !errors.Is(err, app.ErrInvalidStyleLibrary) || s.calls != 0 {
				t.Fatalf("invalid write reached store: calls=%d err=%v", s.calls, err)
			}
		})
	}
}
func TestStyleLibraryNormalizesUnicodeAtAcceptedLimits(t *testing.T) {
	s := &styleLibraryStub{}
	in := styleLibraryInput()
	in.Name = "  " + strings.Repeat("风", 20) + "  "
	in.PromptFragment = strings.Repeat("水", 2000)
	_, _ = app.NewStyleLibrary(s).Change(t.Context(), styleLibraryActor(), in)
	if s.calls != 1 || s.input.Name != strings.TrimSpace(in.Name) || s.input.PromptFragment != in.PromptFragment {
		t.Fatalf("accepted unicode write not preserved: %+v", s.input)
	}
}
func TestStyleLibraryReadsRequireCompleteIdentityAndBoundedQuery(t *testing.T) {
	s := &styleLibraryStub{}
	service := app.NewStyleLibrary(s)
	actor := styleLibraryActor()
	actor.SessionID = uuid.Nil
	if _, err := service.List(t.Context(), actor, app.StyleLibraryFilter{}); err == nil || s.calls != 0 {
		t.Fatalf("missing session reached store: %v", err)
	}
	for _, f := range []app.StyleLibraryFilter{{Limit: 101}, {Limit: -1}, {Category: "unknown"}, {Query: strings.Repeat("风", 101)}} {
		if _, err := service.List(t.Context(), styleLibraryActor(), f); !errors.Is(err, app.ErrInvalidStyleLibrary) {
			t.Fatalf("invalid filter accepted: %+v %v", f, err)
		}
	}
}
