package providerhttp

import (
	"context"
	"sync"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ProtocolDriver encapsulates the communication strategy for a specific AI protocol family.
// Implementations handle request translation, wire protocol execution, async query,
// credential probe discovery, and adapter-specific probe defaults.
type ProtocolDriver interface {
	// Submit translates a generation request and executes the single upstream POST.
	Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error)

	// Query checks the status of an asynchronous task by its ID.
	Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error)

	// Probe performs read-only authentication and model discovery without charging or generating output.
	Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult

	// ProbeModelDefaultParams returns adapter-specific parameter overrides for test credential probes.
	ProbeModelDefaultParams(capability string) map[string]any
}

var (
	registryMu      sync.RWMutex
	defaultRegistry = make(map[string]ProtocolDriver)
)

// RegisterDriver registers a protocol driver for one or more adapter keys in the global registry.
func RegisterDriver(driver ProtocolDriver, adapterKeys ...string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, key := range adapterKeys {
		defaultRegistry[key] = driver
	}
}

// GetDriver retrieves a protocol driver by adapter key from the global registry.
func GetDriver(adapterKey string) (ProtocolDriver, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	driver, ok := defaultRegistry[adapterKey]
	return driver, ok
}
