package main

import (
	"errors"
	"testing"
)

func TestParseRole(t *testing.T) {
	tests := []struct {
		arg     string
		want    role
		wantErr error
	}{
		{arg: "api", want: roleAPI},
		{arg: "worker", wantErr: errRoleNotAvailable},
		{arg: "relay", wantErr: errRoleNotAvailable},
		{arg: "all", wantErr: errRoleNotAvailable},
		{arg: "scheduler", wantErr: errUnknownRole},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got, err := parseRole(tt.arg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseRole(%q) error = %v, want %v", tt.arg, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseRole(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}
