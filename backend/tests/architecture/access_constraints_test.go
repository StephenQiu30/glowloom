package architecture_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func TestAccessModelsDeclareSingleColumnUniquenessAsConstraints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		model     reflect.Type
		fieldName string
	}{
		{name: "user email", model: reflect.TypeOf(model.UserAccount{}), fieldName: "EmailNormalized"},
		{name: "registration email", model: reflect.TypeOf(model.RegistrationVerification{}), fieldName: "EmailNormalized"},
		{name: "registration ticket", model: reflect.TypeOf(model.RegistrationVerification{}), fieldName: "TicketDigest"},
		{name: "session token", model: reflect.TypeOf(model.AuthSession{}), fieldName: "TokenDigest"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			field, ok := testCase.model.FieldByName(testCase.fieldName)
			if !ok {
				t.Fatalf("field %s is not declared", testCase.fieldName)
			}
			markers := strings.Split(field.Tag.Get("gorm"), ";")
			if !containsMarker(markers, "unique") || containsMarker(markers, "uniqueIndex") {
				t.Fatalf("%s.%s must use a single-column unique constraint, got %q", testCase.model.Name(), testCase.fieldName, field.Tag.Get("gorm"))
			}
		})
	}
}

func containsMarker(markers []string, target string) bool {
	for _, marker := range markers {
		if marker == target {
			return true
		}
	}
	return false
}
