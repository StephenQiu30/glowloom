package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsMissingDSN(t *testing.T) {
	_, err := Open(context.Background(), " ")
	if !errors.Is(err, ErrDSNRequired) {
		t.Fatalf("Open() error = %v, want ErrDSNRequired", err)
	}
}

func TestOpenDoesNotExposePassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := Open(ctx, "postgres://probe:canary-secret@127.0.0.1:1/probe?sslmode=disable")
	if err == nil {
		t.Fatal("Open() succeeded against a closed port")
	}
	if strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("Open() error exposed a password: %v", err)
	}
}

func TestOpenWithPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_DB_DSN to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	var got int
	if err := conn.DB.WithContext(ctx).Raw("SELECT 1").Scan(&got).Error; err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if got != 1 {
		t.Errorf("SELECT 1 = %d, want 1", got)
	}
}
