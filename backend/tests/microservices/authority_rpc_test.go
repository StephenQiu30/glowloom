package microservices_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestWriteFenceRPCRejectsMislabeledDatabaseRole(t *testing.T) {
	for _, actual := range []string{"execution", ""} {
		t.Run("database-role-"+actual, func(t *testing.T) {
			database := fenceDatabase(t, actual)
			service, err := fencerpc.NewServer(writefence.WriterMedia, pgfence.New(database), identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
			if err != nil {
				t.Fatal(err)
			}
			client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(startFenceRPC(t, service, nil)))
			if err != nil {
				t.Fatal(err)
			}
			control := writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true}
			if err := client.Install(t.Context(), writefence.WriterMedia, control); !errors.Is(err, writefence.ErrUnavailable) {
				t.Fatalf("mislabeled database acknowledged Media control: %v", err)
			}
			var count int64
			if err := database.WithContext(t.Context()).Table("infra.writer_fence").Where("authority_id = ?", control.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("mislabeled writer changed local records: %d, %v", count, err)
			}
		})
	}
}

func TestAuthorityRPCResumesOriginalCoordinationAfterWriterCommittedButACKWasLost(t *testing.T) {
	identityDB := fenceDatabase(t, "identity")
	stores := make(map[writefence.Writer]*pgfence.Store)
	targets := make(map[writefence.Writer]fencerpc.Target)
	calls := make(map[writefence.Writer]*atomic.Int32)
	var loseMediaACK atomic.Bool
	for _, writer := range writefence.Writers() {
		writerDB := fenceDatabase(t, string(writer))
		store := pgfence.New(writerDB)
		stores[writer] = store
		service, err := fencerpc.NewServer(writer, store, identityFenceToken(writer), creationFenceToken(writer))
		if err != nil {
			t.Fatal(err)
		}
		counter := new(atomic.Int32)
		calls[writer] = counter
		address := startFenceRPC(t, service, func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			counter.Add(1)
			result, err := handler(ctx, request)
			if err == nil && writer == writefence.WriterMedia && loseMediaACK.CompareAndSwap(true, false) {
				return nil, status.Error(codes.Unavailable, "simulated connection failure after commit")
			}
			return result, err
		})
		targets[writer] = fencerpc.Target{Address: address, Token: identityFenceToken(writer), Transport: insecure.NewCredentials()}
	}
	client, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	repository := pgfence.NewAuthorityStore(identityDB)
	coordinator := writefence.NewCoordinator(repository, client)
	authority := writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New()}
	opened, err := coordinator.Set(t.Context(), authority, uuid.New(), false)
	if err != nil || !opened.Complete || opened.Control.Epoch != 1 {
		t.Fatalf("open: %+v, %v", opened, err)
	}
	command := fenceCommand()
	command.Authorities = []writefence.Authority{opened.Control.Authority}
	// A real issuer policy transaction would do its policy checks before Register.
	// This test proves only the technical registration/writer handoff, not policy.
	if err := identityDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return repository.Register(t.Context(), tx, command, writefence.WriterMedia)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stores[writefence.WriterMedia].Admit(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	pending := command
	pending.ID = uuid.New()
	if _, err := stores[writefence.WriterExecution].Admit(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	mediaDB := fenceDatabase(t, "media")
	if err := mediaDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return stores[writefence.WriterMedia].Complete(t.Context(), tx, command, json.RawMessage(`{"asset":"published-before-revocation","sequence":9007199254740993}`))
	}); err != nil {
		t.Fatal(err)
	}
	loseMediaACK.Store(true)
	authority = opened.Control.Authority
	coordinationID := uuid.New()
	partial, err := coordinator.Set(t.Context(), authority, coordinationID, true)
	if !errors.Is(err, writefence.ErrUnavailable) || partial.Complete || len(partial.Acknowledged) != 2 {
		t.Fatalf("lost final status became success: %+v, %v", partial, err)
	}
	// Local admission is closed even while one writer's ACK remains unknown.
	late := command
	late.ID = uuid.New()
	if err := identityDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return repository.Register(t.Context(), tx, late, writefence.WriterExecution)
	}); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("partial revocation left new admission open: %v", err)
	}
	// A fresh process can resume only the original control, not a replacement ID.
	restartedClient, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	restarted := writefence.NewCoordinator(pgfence.NewAuthorityStore(identityDB), restartedClient)
	blocked, err := restarted.Set(t.Context(), authority, coordinationID, true)
	if err != nil || !blocked.Complete || blocked.Control.Epoch != 2 {
		t.Fatalf("original coordination did not resume: %+v, %v", blocked, err)
	}
	for _, writer := range writefence.Writers() {
		want := int32(2)
		if writer == writefence.WriterMedia {
			want = 3
		}
		if calls[writer].Load() != want {
			t.Fatalf("ACK replay at %s: got %d, want %d", writer, calls[writer].Load(), want)
		}
	}
	executionDB := fenceDatabase(t, "execution")
	if err := executionDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return stores[writefence.WriterExecution].Guard(t.Context(), tx, pending)
	}); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("old late command retained publication rights: %v", err)
	}
	record, err := restartedClient.Resolve(t.Context(), writefence.WriterMedia, command)
	if err != nil || record.State != writefence.Committed || string(record.Receipt) != `{"asset":"published-before-revocation","sequence":9007199254740993}` {
		t.Fatalf("committed original outcome was lost: %+v, %v", record, err)
	}
	reopened, err := restarted.Set(t.Context(), blocked.Control.Authority, uuid.New(), false)
	if err != nil || !reopened.Complete || reopened.Control.Epoch != 3 {
		t.Fatalf("reopen: %+v, %v", reopened, err)
	}
	if err := executionDB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		return stores[writefence.WriterExecution].Guard(t.Context(), tx, pending)
	}); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("reopen resurrected old publication: %v", err)
	}
}
