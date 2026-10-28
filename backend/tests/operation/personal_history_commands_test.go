package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type historyCommandStore struct {
	calls int
	input app.PersonalHistoryCommand
}

func (s *historyCommandStore) ApplyPersonalHistoryCommand(_ context.Context, _ identity.Principal, _ uuid.UUID, in app.PersonalHistoryCommand) (app.PersonalHistoryCommandReceipt, error) {
	s.calls++
	s.input = in
	return app.PersonalHistoryCommandReceipt{}, nil
}
func TestPersonalHistoryCommandRejectsAmbiguousTargetsAndMissingIntent(t *testing.T) {
	op, out := uuid.New(), uuid.New()
	yes := true
	for _, in := range []app.PersonalHistoryCommand{
		{Action: "favorite", Targets: []app.PersonalHistoryTarget{{OperationID: op, OutputID: &out}}},
		{Action: "delete", Favorited: &yes, Targets: []app.PersonalHistoryTarget{{OperationID: op}}},
		{Action: "delete", Targets: []app.PersonalHistoryTarget{{OperationID: op}, {OperationID: op, OutputID: &out}}},
		{Action: "delete", Targets: []app.PersonalHistoryTarget{{OperationID: op, ExpectedRevision: -1}}},
		{Action: "delete", Targets: []app.PersonalHistoryTarget{{OperationID: op, OutputID: &out}, {OperationID: op, OutputID: &out}}},
	} {
		s := &historyCommandStore{}
		_, err := app.NewPersonalHistoryCommander(s).Apply(t.Context(), historyActor(), uuid.New(), in)
		if !errors.Is(err, app.ErrInvalidPublicQuery) || s.calls != 0 {
			t.Fatalf("accepted invalid command %+v: %v", in, err)
		}
	}
	s := &historyCommandStore{}
	_, err := app.NewPersonalHistoryCommander(s).Apply(t.Context(), historyActor(), uuid.New(), app.PersonalHistoryCommand{Action: "favorite", Favorited: &yes, Targets: []app.PersonalHistoryTarget{{OperationID: op, OutputID: &out}}})
	if err != nil || s.calls != 1 {
		t.Fatal(err)
	}
}
func TestPersonalHistoryAdvancedFiltersNormalizeAndRejectDuplicates(t *testing.T) {
	in := app.PersonalHistoryInput{ProcessTypes: []string{"super_resolution", "frame_interpolation"}, Resolutions: []string{"4K", "2K"}, Ratios: []string{"21:9", "1:1"}, GenerationModes: []string{"tts", "song"}}
	if err := in.Normalize(); err != nil || in.Resolutions[0] != "2K" {
		t.Fatalf("normalize %+v: %v", in, err)
	}
	for _, in := range []app.PersonalHistoryInput{{Resolutions: []string{"2K", "2K"}}, {Resolutions: []string{"garbage"}}, {Ratios: []string{"1:0"}}, {GenerationModes: []string{"invented"}}} {
		if !errors.Is(in.Normalize(), app.ErrInvalidPublicQuery) {
			t.Fatalf("accepted %+v", in)
		}
	}
}

func TestPersonalHistoryCommandJSONRequiresExplicitRevision(t *testing.T) {
	id := uuid.NewString()
	for _, raw := range []string{`{"operation_id":"` + id + `"}`, `{"operation_id":"` + id + `","expected_revision":null}`, `{"operation_id":"` + id + `","expected_revision":0,"unknown":true}`} {
		var target app.PersonalHistoryTarget
		if json.Unmarshal([]byte(raw), &target) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var target app.PersonalHistoryTarget
	if err := json.Unmarshal([]byte(`{"operation_id":"`+id+`","output_id":null,"expected_revision":0}`), &target); err != nil || target.ExpectedRevision != 0 {
		t.Fatal(err)
	}
}
