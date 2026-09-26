// Package temporalconn owns the Temporal workflow client used by backend roles.
package temporalconn

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

var (
	// ErrAddrRequired means no Temporal service address is configured.
	ErrAddrRequired = errors.New("LV_TEMPORAL_ADDR is required")
	// ErrNamespaceRequired means no Temporal namespace is configured.
	ErrNamespaceRequired = errors.New("LV_TEMPORAL_NAMESPACE is required")
)

// Connection owns a Temporal client and the configured namespace.
type Connection struct {
	Client    client.Client
	namespace string
}

// Open creates a lazy client so a transient Temporal outage does not stop the API.
func Open(addr, namespace string, logger *zap.Logger) (*Connection, error) {
	addr = strings.TrimSpace(addr)
	namespace = strings.TrimSpace(namespace)
	if addr == "" {
		return nil, ErrAddrRequired
	}
	if namespace == "" {
		return nil, ErrNamespaceRequired
	}
	workflowClient, err := client.NewLazyClient(client.Options{
		HostPort:  addr,
		Namespace: namespace,
		Logger:    sdkLogger{logger: logger.Sugar()},
	})
	if err != nil {
		return nil, fmt.Errorf("create Temporal client: %w", err)
	}
	return &Connection{Client: workflowClient, namespace: namespace}, nil
}

// Ping checks the service and confirms that this process's namespace exists.
func (c *Connection) Ping(ctx context.Context) error {
	if _, err := c.Client.CheckHealth(ctx, nil); err != nil {
		return fmt.Errorf("check Temporal health: %w", err)
	}
	if _, err := c.Client.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: c.namespace,
	}); err != nil {
		return fmt.Errorf("describe Temporal namespace: %w", err)
	}
	return nil
}

// Close releases the Temporal client connection.
func (c *Connection) Close() {
	c.Client.Close()
}

type sdkLogger struct {
	logger *zap.SugaredLogger
}

func (l sdkLogger) Debug(msg string, keyvals ...any) { l.logger.Debugw(msg, keyvals...) }
func (l sdkLogger) Info(msg string, keyvals ...any)  { l.logger.Infow(msg, keyvals...) }
func (l sdkLogger) Warn(msg string, keyvals ...any)  { l.logger.Warnw(msg, keyvals...) }
func (l sdkLogger) Error(msg string, keyvals ...any) { l.logger.Errorw(msg, keyvals...) }
