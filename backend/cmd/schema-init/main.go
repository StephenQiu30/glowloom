package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stephenqiu30/lanverse/backend/internal/platform/database"
)

func main() {
	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	schema, err := os.ReadFile("schema/current.sql")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read current schema failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		fmt.Fprintf(os.Stderr, "apply current schema failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("current schema is ready")
}
