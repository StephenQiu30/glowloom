package objectstorage

import (
	"context"
	"errors"
)

// Exists checks one exact private key and distinguishes absence from access/network failures.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Stat(ctx, key)
	if errors.Is(err, ErrObjectMissing) {
		return false, nil
	}
	return err == nil, err
}
