package store

import "context"

// Open returns the configured Store (Postgres or in-memory).
func Open(ctx context.Context) (Store, error) {
	return OpenFromEnv(ctx)
}
