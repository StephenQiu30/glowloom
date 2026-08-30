package platform_test

import (
	"context"
	"database/sql"
	"io"
	"os"
	"testing"
	"time"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
)

func TestDatabasePoolRetainsItsBoundedCapacityForBurstReuse(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL pool journey")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = platformdatabase.Close(database)
		}
	})
	connectionPool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	capacity := connectionPool.Stats().MaxOpenConnections
	if capacity < 1 {
		t.Fatalf("database pool capacity = %d", capacity)
	}

	for wave := 1; wave <= 2; wave++ {
		connections := reservePoolCapacity(t, ctx, connectionPool, capacity)
		if open := connectionPool.Stats().OpenConnections; open != capacity {
			closeReservedConnections(t, connections)
			t.Fatalf("wave %d open connections = %d, want %d", wave, open, capacity)
		}
		closeReservedConnections(t, connections)
	}

	stats := connectionPool.Stats()
	if stats.MaxIdleClosed != 0 {
		t.Fatalf("pool closed %d reusable connections after bounded bursts", stats.MaxIdleClosed)
	}
	if stats.OpenConnections != capacity {
		t.Fatalf("retained connections = %d, want bounded capacity %d", stats.OpenConnections, capacity)
	}
	if err = platformdatabase.Close(database); err != nil {
		t.Fatal(err)
	}
	closed = true
	if open := connectionPool.Stats().OpenConnections; open != 0 {
		t.Fatalf("connections after close = %d", open)
	}
}

func reservePoolCapacity(t *testing.T, ctx context.Context, pool *sql.DB, capacity int) []*sql.Conn {
	t.Helper()
	connections := make([]*sql.Conn, 0, capacity)
	for range capacity {
		connection, err := pool.Conn(ctx)
		if err != nil {
			closeReservedConnections(t, connections)
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	return connections
}

func closeReservedConnections(t *testing.T, connections []*sql.Conn) {
	t.Helper()
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
