package microservices_test

import (
	"errors"
	"testing"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
)

func TestFrozenInputRejectsIncompleteContract(t *testing.T) {
	var input promptdomain.FrozenInput
	if !errors.Is(input.Validate(), promptdomain.ErrFrozenInputInvalid) {
		t.Fatal("an incomplete frozen input must fail closed")
	}
}
