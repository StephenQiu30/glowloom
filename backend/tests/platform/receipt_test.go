package platform_test

import (
	"testing"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

func TestReceiptHashAndReplayAreDeterministic(t *testing.T) {
	t.Parallel()
	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		ID string `json:"id"`
	}
	first, err := platformcommand.InputHash(input{Name: "项目"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := platformcommand.InputHash(input{Name: "项目"})
	if err != nil || first != second {
		t.Fatalf("hash mismatch: %q %q %v", first, second, err)
	}
	encoded, err := platformcommand.Result(output{ID: "project-1"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := platformcommand.Replay[output](platformcommand.Receipt{InputHash: first, Result: encoded}, first)
	if err != nil || replayed.ID != "project-1" {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	if _, err = platformcommand.Replay[output](platformcommand.Receipt{InputHash: first, Result: encoded}, "different"); err == nil {
		t.Fatal("receipt accepted a different input hash")
	}
}
