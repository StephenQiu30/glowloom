package command

import "testing"

func TestReceiptHashAndReplayAreDeterministic(t *testing.T) {
	t.Parallel()
	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		ID string `json:"id"`
	}
	first, err := InputHash(input{Name: "项目"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := InputHash(input{Name: "项目"})
	if err != nil || first != second {
		t.Fatalf("hash mismatch: %q %q %v", first, second, err)
	}
	encoded, err := Result(output{ID: "project-1"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := Replay[output](Receipt{InputHash: first, Result: encoded}, first)
	if err != nil || replayed.ID != "project-1" {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	if _, err = Replay[output](Receipt{InputHash: first, Result: encoded}, "different"); err == nil {
		t.Fatal("receipt accepted a different input hash")
	}
}
