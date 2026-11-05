package objectstorage

import (
	"context"
	"errors"
	"strings"
)

// ErrInventoryLimit means an exact inventory cannot fit the caller's explicit budget.
var ErrInventoryLimit = errors.New("object inventory exceeds limit")

// ObjectMetadata is actual private listing metadata; callers own scope selection.
type ObjectMetadata struct {
	Ref  Ref
	Key  string
	Size int64
}

// ListMetadata reads one concrete private prefix and never returns a partial inventory.
// The caller must authorize and build the prefix; this platform client has no user context.
func (c *Client) ListMetadata(ctx context.Context, prefix string, maxItems int) ([]ObjectMetadata, error) {
	if c == nil || c.native.sdk == nil {
		return nil, ErrUnavailable
	}
	if maxItems < 1 || maxItems > 50000 || !strings.HasSuffix(prefix, "/") || strings.Count(prefix, "/") < 2 || !validKey(strings.TrimSuffix(prefix, "/")) || strings.ContainsAny(prefix, "\x00\r\n") {
		return nil, ErrInvalidObject
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := c.native.list(ctx, prefix, maxItems)
	if err != nil {
		return nil, storageError("list", "direct", err)
	}
	return validateInventory(out, prefix, maxItems)
}
