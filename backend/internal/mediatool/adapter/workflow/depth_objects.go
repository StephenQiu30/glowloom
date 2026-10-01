package workflow

import "context"

// Remove deletes only an exact unpublished key fenced by DepthWorkerStore.
func (o *Objects) Remove(ctx context.Context, key string) error { return o.client.Remove(ctx, key) }
