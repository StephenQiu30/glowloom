package app

import (
	"errors"
	"fmt"
)

// Role identifies a backend process role selected by the command-line entrypoint.
type Role string

const (
	// RoleAPI selects the API process.
	RoleAPI Role = "api"
	// RoleWorker selects the Temporal worker process.
	RoleWorker Role = "worker"
	// RoleRelay selects the Kafka relay process.
	RoleRelay Role = "relay"
	// RoleAll selects all backend roles in one process.
	RoleAll Role = "all"
)

var (
	// ErrUnknownRole means a requested role is not recognized.
	ErrUnknownRole = errors.New("unknown role")
	// ErrRoleNotAvailable means a recognized role has not been implemented yet.
	ErrRoleNotAvailable = errors.New("role not available yet")
)

// ParseRole validates a role requested by the command-line entrypoint.
func ParseRole(value string) (Role, error) {
	role := Role(value)
	switch role {
	case RoleAPI, RoleWorker, RoleRelay, RoleAll:
		return role, nil
	default:
		return "", fmt.Errorf("%w: %q, want api|worker|relay|all", ErrUnknownRole, value)
	}
}

// WorkerQueue accepts the implemented flow queue and rejects work that cannot run yet.
func WorkerQueue(value string) (string, error) {
	if value == "" || value == "flow" {
		return "flow", nil
	}
	return "", fmt.Errorf("%w: worker queue %q", ErrRoleNotAvailable, value)
}
