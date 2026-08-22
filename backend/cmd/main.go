package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/segmentio/kafka-go"

	"github.com/stephenqiu30/lanverse/backend/src/agents"
	"github.com/stephenqiu30/lanverse/backend/src/generationplanning"
	"github.com/stephenqiu30/lanverse/backend/src/identity"
	"github.com/stephenqiu30/lanverse/backend/src/platform/agent"
	"github.com/stephenqiu30/lanverse/backend/src/platform/coordination"
	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/messaging"
	"github.com/stephenqiu30/lanverse/backend/src/platform/objectstorage"
	platformruntime "github.com/stephenqiu30/lanverse/backend/src/platform/runtime"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
	"github.com/stephenqiu30/lanverse/backend/src/scripts"
)

// One process entrypoint owns every Go runtime role. LANVERSE_ROLE selects the
// role; this keeps deployment and local startup deterministic without creating
// a parallel cmd tree or duplicate wiring.
func main() {
	role := toolkit.EnvOr("LANVERSE_ROLE", "api")
	switch role {
	case "api":
		runAPI()
	case "operation-worker":
		runOperationWorker()
	case "import-worker", "provider-worker", "media-worker":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		platformruntime.RunIdleWorker(ctx, role)
	case "schema-init":
		runSchemaInit()
	default:
		logFatal("unsupported LANVERSE_ROLE", fmt.Errorf("%s", role))
	}
}

func runAPI() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.Connect(ctx)
	if err != nil {
		logFatal("database connection failed", err)
	}
	defer pool.Close()
	orm, err := database.OpenGORM(pool)
	if err != nil {
		logFatal("gorm database initialization failed", err)
	}
	authConfig, err := identity.AuthConfigFromEnv()
	if err != nil {
		logFatal("identity configuration failed", err)
	}
	jwtManager, err := identity.NewJWTManagerFromEnv()
	if err != nil {
		logFatal("JWT configuration failed", err)
	}
	redisCoordinator := coordination.NewRedisCoordinator()
	if err := redisCoordinator.Ping(ctx); err != nil {
		logFatal("redis connection failed", err)
	}
	defer redisCoordinator.Close()
	storage, err := objectstorage.NewMinIOObjectStore(ctx)
	if err != nil {
		logFatal("object storage connection failed", err)
	}
	scriptService := scripts.NewScriptAnalysisService(scripts.NewScriptRepository(orm, storage))
	agentService := agents.NewAgentService(agents.NewAgentRepository(orm), agent.NewAgentHTTPClient())
	identityService := identity.NewIdentityService(identity.NewIdentityRepository(orm, authConfig.RefreshTTL), redisCoordinator, jwtManager, authConfig)
	generationService := generationplanning.NewGenerationPlanService(generationplanning.NewGenerationPlanRepository(orm))
	root := chi.NewRouter()
	root.Use(httpapi.RecoverMiddleware)
	root.Use(httpapi.RequestIDMiddleware)
	root.Use(corsMiddleware)
	root.Use(identity.RequireForBusiness(identityService))
	scripts.NewScriptController(scriptService).Mount(root)
	agents.NewAgentController(agentService).Mount(root)
	generationplanning.NewGenerationPlanController(generationService).Mount(root)
	identity.NewIdentityController(identityService).Mount(root)
	server := &http.Server{Addr: toolkit.EnvOr("API_ADDR", "127.0.0.1:8686"), Handler: root, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("lanverse api listening", "addr", server.Addr, "storage_bucket", storage.Bucket())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logFatal("api server failed", err)
	}
}

func runOperationWorker() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.Connect(ctx)
	if err != nil {
		logFatal("database connection failed", err)
	}
	defer pool.Close()
	storage, err := objectstorage.NewMinIOObjectStore(ctx)
	if err != nil {
		logFatal("object storage connection failed", err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		logFatal("gorm database initialization failed", err)
	}
	repository := scripts.NewScriptRepository(orm, storage)
	writer := &kafka.Writer{Addr: kafka.TCP(messaging.Brokers()...), Topic: messaging.OperationTaskTopic, Balancer: &kafka.Hash{}, AllowAutoTopicCreation: true, BatchTimeout: 100 * time.Millisecond}
	defer writer.Close()
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: messaging.Brokers(), GroupID: "lanverse-operation-worker", Topic: messaging.OperationTaskTopic, MinBytes: 1, MaxBytes: 10e6, StartOffset: kafka.FirstOffset, CommitInterval: 500 * time.Millisecond})
	defer reader.Close()
	go publishOutbox(ctx, repository, writer)
	slog.Info("lanverse operation worker started", "brokers", messaging.Brokers(), "topic", messaging.OperationTaskTopic)
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("fetch kafka message failed", "error", err)
			continue
		}
		messageID := fmt.Sprintf("%s/%d/%d", message.Topic, message.Partition, message.Offset)
		seen, err := repository.HasInboxMessage(ctx, messageID)
		if err != nil {
			slog.Error("check inbox failed", "error", err)
			continue
		}
		if !seen {
			var request scripts.AnalysisRequest
			if err := json.Unmarshal(message.Value, &request); err != nil {
				slog.Error("decode analysis request failed", "error", err)
				continue
			}
			if err := repository.ProcessAnalysis(ctx, request); err != nil {
				slog.Error("process analysis failed", "operation_id", request.OperationID, "error", err)
				continue
			}
			if err := repository.RecordInboxMessage(ctx, messageID, message.Topic); err != nil {
				slog.Error("record inbox failed", "error", err)
				continue
			}
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			slog.Error("commit kafka message failed", "error", err)
		}
	}
}

func publishOutbox(ctx context.Context, repository *scripts.ScriptRepository, writer *kafka.Writer) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := repository.PendingOutbox(ctx, 20)
			if err != nil {
				slog.Error("list outbox failed", "error", err)
				continue
			}
			for _, event := range events {
				if err := writer.WriteMessages(ctx, kafka.Message{Key: []byte(event.Key), Value: event.Payload}); err != nil {
					slog.Error("publish outbox failed", "event_id", event.ID, "error", err)
					continue
				}
				if err := repository.MarkOutboxPublished(ctx, event.ID); err != nil {
					slog.Error("mark outbox published failed", "event_id", event.ID, "error", err)
				}
			}
		}
	}
}

func runSchemaInit() {
	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		logFatal("database connection failed", err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("schema/current.sql")
	if err != nil {
		logFatal("read current schema failed", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		logFatal("apply current schema failed", err)
	}
	if err := database.VerifyCurrent(ctx, pool); err != nil {
		logFatal("current schema verification failed", err)
	}
	fmt.Println("current schema is ready")
}

func logFatal(message string, err error) { slog.Error(message, "error", err); os.Exit(1) }
func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigin := toolkit.EnvOr("FRONTEND_ORIGIN", "http://127.0.0.1:8123")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Workspace-Id, Idempotency-Key, X-Request-Idempotency-Key, If-Match, X-Request-Id, Traceparent")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if origin == "" || origin != allowedOrigin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
