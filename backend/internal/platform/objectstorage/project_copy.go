package objectstorage

import (
	"context"
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
)

// Exists checks one exact private key and distinguishes absence from access/network failures.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if c == nil || (c.sdk == nil && c.native == nil) {
		return false, ErrUnavailable
	}
	if !validKey(key) {
		return false, ErrInvalidObject
	}
	if c.native != nil {
		_, err := c.Stat(ctx, key)
		if errors.Is(err, ErrObjectMissing) {
			return false, nil
		}
		return err == nil, err
	}
	_, err := c.sdk.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if response := minio.ToErrorResponse(err); response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
			return false, nil
		}
		return false, fmt.Errorf("check private object existence: %w", err)
	}
	return true, nil
}
