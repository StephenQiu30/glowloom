package domain

import "testing"

func TestEqualJSONPersistedNumericSemantics(t *testing.T) {
	for _, test := range []struct {
		name, left, right string
		equal             bool
	}{
		{"jsonb exponent", `{"x":[1e-7,-4.25E+3]}`, `{"x":[0.0000001,-4250.00]}`, true},
		{"negative zero", `-0`, `0.0000e999999`, true},
		{"large integer same", `9007199254740993`, `9007199254740993.0`, true},
		{"large integer different", `9007199254740992`, `9007199254740993`, false},
		{"decimal difference", `0.10000000000000000001`, `0.1`, false},
		{"large exponent bounded", `1e999999999`, `10e999999998`, true},
		{"overflow exponent fail closed", `1e9223372036854775808`, `10e9223372036854775807`, false},
		{"type distinct", `1`, `"1"`, false},
		{"missing distinct", `{"x":null}`, `{}`, false},
		{"array order", `[1,2]`, `[2,1]`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := equalJSON([]byte(test.left), []byte(test.right)); got != test.equal {
				t.Fatalf("equalJSON=%v want %v", got, test.equal)
			}
		})
	}
}
