package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
)

// ErrInventoryLimit means an exact inventory cannot fit the caller's explicit budget.
var ErrInventoryLimit = errors.New("object inventory exceeds limit")

// ObjectMetadata is actual private listing metadata; callers own scope selection.
type ObjectMetadata struct {
	Key  string
	Size int64
}

// ListMetadata reads one concrete private prefix and never returns a partial inventory.
// The caller must authorize and build the prefix; this platform client has no user context.
func (c *Client) ListMetadata(ctx context.Context, prefix string, maxItems int) ([]ObjectMetadata, error) {
	if c == nil || c.sdk == nil || maxItems < 1 || maxItems > 50000 || !strings.HasSuffix(prefix, "/") || strings.Count(prefix, "/") < 3 || !validKey(strings.TrimSuffix(prefix, "/")) || strings.ContainsAny(prefix, "\x00\r\n") {
		return nil, ErrInvalidObject
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]ObjectMetadata, 0, min(maxItems, 1000))
	seen := make(map[string]struct{})
	for object := range c.sdk.ListObjectsIter(readCtx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true, MaxKeys: min(maxItems+1, 1000)}) {
		if object.Err != nil {
			return nil, fmt.Errorf("read private object inventory: %w", object.Err)
		}
		if object.Size < 0 || !validKey(object.Key) || !strings.HasPrefix(object.Key, prefix) {
			return nil, ErrInvalidObject
		}
		if _, exists := seen[object.Key]; exists {
			return nil, ErrInvalidObject
		}
		seen[object.Key] = struct{}{}
		if len(out) == maxItems {
			return nil, ErrInventoryLimit
		}
		out = append(out, ObjectMetadata{Key: object.Key, Size: object.Size})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
