package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ReadSnapshot retries a complete authorized read after a serialization conflict.
// Each attempt must rebuild its result; run must not perform business writes or
// external side effects. Five attempts use bounded backoff to outlast bursts of
// session activity. Other errors are returned without retrying.
func ReadSnapshot(ctx context.Context, database *gorm.DB, run func(*gorm.DB) error) error {
	var err error
	for attempt := range 5 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = database.WithContext(ctx).Transaction(run, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		var serialization *pgconn.PgError
		if !errors.As(err, &serialization) || serialization.Code != "40001" {
			return err
		}
		if attempt == 4 {
			break
		}
		timer := time.NewTimer(10 * time.Millisecond << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
