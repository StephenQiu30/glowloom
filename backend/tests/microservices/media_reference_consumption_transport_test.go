package microservices_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpcpinproof"
	creationapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/creationpinproof"
)

type pinConsumptionRead func(context.Context, creationapp.ReferencePinConsumptionKey) (creationapp.ReferencePinConsumption, error)

func (f pinConsumptionRead) ReadReferencePinConsumption(ctx context.Context, key creationapp.ReferencePinConsumptionKey) (creationapp.ReferencePinConsumption, error) {
	return f(ctx, key)
}

func TestMediaReferenceConsumptionPrivateAudienceCannotBeSubstituted(t *testing.T) {
	f, _, pin := frozenPinReceiptFixture(t)
	f.store = creationStoreWithPin(t, f, pin)
	f.freeze(t)
	var calls atomic.Int32
	reader := pinConsumptionRead(func(ctx context.Context, key creationapp.ReferencePinConsumptionKey) (creationapp.ReferencePinConsumption, error) {
		calls.Add(1)
		return f.store.ReadReferencePinConsumption(ctx, key)
	})
	serverTransport, clientTransport, wrongPeer := ownerPeerTLS(t, "media")
	token := strings.Repeat("creation-pin-consumer-fixture", 2)
	server, err := grpcpinproof.NewServer(reader, token, "media", serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	for _, name := range []string{"certificate", "token"} {
		t.Run(name, func(t *testing.T) {
			target := creationpinproof.Target{Address: address, Token: token, Transport: clientTransport}
			if name == "certificate" {
				target.Transport = wrongPeer
			} else {
				target.Token = strings.Repeat("wrong-audience-fixture", 3)
			}
			client, err := creationpinproof.New(target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if _, err := client.VerifyAdoption(ctx, pin); err == nil {
				t.Fatal("wrong private audience obtained a source proof")
			}
			if calls.Load() != 0 {
				t.Fatal("unauthenticated peer reached source persistence")
			}
		})
	}
	client, err := creationpinproof.New(creationpinproof.Target{Address: address, Token: token, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.VerifyAdoption(t.Context(), pin); err != nil || calls.Load() != 1 {
		t.Fatal("correct service audience could not recover exact adoption", err, calls.Load())
	}
}
