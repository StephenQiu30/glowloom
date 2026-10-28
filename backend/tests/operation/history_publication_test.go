package operation_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type publicationFake struct{ calls int }

func (s *publicationFake) PublishHistory(context.Context, identity.Principal, uuid.UUID, app.HistoryPublicationInput) (app.HistoryPublicationReceipt, error) {
	s.calls++
	return app.HistoryPublicationReceipt{}, nil
}
func TestHistoryPublicationRejectsAmbiguousTargets(t *testing.T) {
	s := &publicationFake{}
	command := app.NewHistoryPublisher(s)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	target := app.HistoryPublicationTarget{OperationID: uuid.New(), OutputID: uuid.New()}
	for _, in := range []app.HistoryPublicationInput{
		{Title: "作品"},
		{Title: "作品", Description: "invalid\x00description", Targets: []app.HistoryPublicationTarget{target}},
		{Title: " ", Targets: []app.HistoryPublicationTarget{target}},
		{Title: "作品", Targets: []app.HistoryPublicationTarget{target, target}},
		{Title: "作品", Targets: []app.HistoryPublicationTarget{{OperationID: target.OperationID}}},
	} {
		if _, err := command.Publish(context.Background(), actor, uuid.New(), in); err == nil {
			t.Fatal("invalid publication accepted")
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid publication reached persistence")
	}
	in := app.HistoryPublicationInput{Title: "作品", Targets: []app.HistoryPublicationTarget{target}}
	if _, err := command.Publish(context.Background(), actor, uuid.New(), in); err != nil {
		t.Fatal(err)
	}
	if s.calls != 1 {
		t.Fatalf("calls=%d", s.calls)
	}
}
