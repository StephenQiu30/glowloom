package creation_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/agenthttp"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

// The Python native-runtime test supplies a short-lived real Agent HTTP process.
func TestNativeAgentCommandAcceptance(t *testing.T) {
	endpoint := os.Getenv("LANVERSE_TEST_CREATION_URL")
	if endpoint == "" {
		t.Skip("requires the native Agent acceptance test process")
	}
	var command domain.Command
	if err := json.Unmarshal([]byte(os.Getenv("LANVERSE_TEST_CREATION_COMMAND")), &command); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{Command: command, Endpoint: endpoint}
	var err error
	run.PayloadHash, err = app.PayloadHash(command)
	if err != nil {
		t.Fatal(err)
	}
	client, err := adapter.New(adapter.Config{Secret: os.Getenv("LANVERSE_TEST_CREATION_SECRET")}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err = client.Lookup(ctx, run); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("initial lookup: %v", err)
	}
	receipt, err := client.Accept(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := client.Accept(ctx, run)
	if err != nil || replayed.ReceiptID != receipt.ReceiptID || !replayed.AcceptedAt.Equal(receipt.AcceptedAt) {
		t.Fatalf("acceptance replay did not preserve receipt: %v", err)
	}
	lookedUp, err := client.Lookup(ctx, run)
	if err != nil || lookedUp.ReceiptID != receipt.ReceiptID {
		t.Fatalf("lookup did not preserve receipt: %v", err)
	}
}
