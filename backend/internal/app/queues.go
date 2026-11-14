package app

import (
	"fmt"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ResolveWorkerQueues expands auto to the queues used by the combined role.
// Provider tasks require configured signing capability; Codex remains opt-in.
// An empty request preserves the existing standalone worker's flow-only default.
func ResolveWorkerQueues(value string, cfg config.Config) (string, error) {
	if value == "auto" {
		value = "flow,media"
		if cfg.HasCredentialPrivateKey() {
			value += ",provider"
		}
	}
	return WorkerQueue(value)
}

// WorkerQueue returns the canonical set of implemented backend queues.
func WorkerQueue(value string) (string, error) {
	if value == "" {
		return "flow", nil
	}
	var flow, media, provider, codex bool
	for _, item := range strings.Split(value, ",") {
		switch strings.TrimSpace(item) {
		case "agent.codex":
			if codex {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			codex = true
		case "flow":
			if flow {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			flow = true
		case "provider":
			if provider {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			provider = true
		case "media":
			if media {
				return "", fmt.Errorf("%w: duplicate worker queue %q", ErrRoleNotAvailable, item)
			}
			media = true
		default:
			return "", fmt.Errorf("%w: worker queue %q", ErrRoleNotAvailable, item)
		}
	}
	var selected []string
	if flow {
		selected = append(selected, "flow")
	}
	if media {
		selected = append(selected, "media")
	}
	if provider {
		selected = append(selected, "provider")
	}
	if codex {
		selected = append(selected, "agent.codex")
	}
	return strings.Join(selected, ","), nil
}
