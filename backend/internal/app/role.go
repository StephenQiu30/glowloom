package app

import (
	"errors"
	"fmt"
	"strings"
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

// WorkerQueue returns the canonical set of implemented backend queues.
func WorkerQueue(value string) (string, error) {
	if value == "" {
		return "flow", nil
	}
	var flow, media bool
	for _, item := range strings.Split(value, ",") {
		switch strings.TrimSpace(item) {
		case "flow":
			if flow {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			flow = true
		case "media":
			if media {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			media = true
		default:
			return "", fmt.Errorf("%w: worker queue %q", ErrRoleNotAvailable, item)
		}
	}
	if flow && media {
		return "flow,media", nil
	}
	if media {
		return "media", nil
	}
	return "flow", nil
}
