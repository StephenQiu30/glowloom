package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	ep "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func runRAG(args []string) error {
	usage := errors.New("usage: glowloom rag status|disable --project UUID | enable --project UUID --actor UUID --credits N | eval --project UUID --actor UUID --trial UUID --cases FILE")
	if len(args) == 0 {
		return usage
	}
	f := flag.NewFlagSet("rag", flag.ContinueOnError)
	var project, actor uuid.UUID
	f.TextVar(&project, "project", uuid.Nil, "project UUID")
	var credits int64
	var trial uuid.UUID
	var casesFile string
	switch args[0] {
	case "enable":
		f.TextVar(&actor, "actor", uuid.Nil, "authorized indexing actor")
		f.Int64Var(&credits, "credits", 0, "cumulative embedding credit allowance")
	case "eval":
		f.TextVar(&actor, "actor", uuid.Nil, "authorized project indexing actor")
		f.TextVar(&trial, "trial", uuid.Nil, "unique evaluation trial")
		f.StringVar(&casesFile, "cases", "", "labelled JSON cases")
	case "disable", "status":
	default:
		return usage
	}
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || project == uuid.Nil || args[0] == "enable" && (actor == uuid.Nil || credits <= 0) {
		return usage
	}
	timeout := 30 * time.Second
	if args[0] == "eval" {
		if actor == uuid.Nil || trial == uuid.Nil || casesFile == "" {
			return usage
		}
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := platformdb.Open(ctx, os.Getenv("GLOWLOOM_RAG_OPERATOR_DB_DSN"), noop.NewTracerProvider())
	if err != nil {
		return errors.New("open explicit GLOWLOOM_RAG_OPERATOR_DB_DSN failed")
	}
	defer func() { _ = conn.Close() }()
	if args[0] == "eval" {
		return runRAGEval(ctx, conn.DB, project, actor, trial, casesFile)
	}
	if args[0] != "status" {
		return aigateway.ConfigureEvidence(ctx, conn.DB, project, actor, credits, args[0] == "enable")
	}
	var out struct {
		Enabled, Ready                         bool
		ModelKey                               string
		Generation, ReleaseID, ActiveReleaseID *uuid.UUID
		LimitCredits, Held, Spent, Indexed     int64
		ActiveProfile                          string
		UnknownRequests                        int64
		WorkerFailed                           bool
	}
	err = conn.DB.WithContext(ctx).Raw(`SELECT coalesce(p.enabled,false) AS enabled,coalesce(pr.default_models->>'text.embed','') AS model_key,
 coalesce(p.enabled AND NOT pr.is_delete AND pr.status='active' AND p.active_release_id=m.id AND p.active_profile=?,false) AS ready,
 p.generation,m.id AS release_id,p.active_release_id,p.active_profile,p.limit_credits,coalesce(b.held,0) AS held,coalesce(b.spent,0) AS spent,
 (SELECT count(*) FROM workspace.evidence_embedding v WHERE v.project_id=p.project_id AND v.release_id=m.id) AS indexed,
 (SELECT count(*) FROM ai.request r WHERE r.project_id=p.project_id AND r.run_id=p.budget_id AND r.state='unknown') AS unknown_requests,
 EXISTS(SELECT 1 FROM infra.task_work w WHERE w.business_id=p.generation::text AND w.failed_at IS NOT NULL) AS worker_failed
 FROM workspace.project pr LEFT JOIN workspace.evidence_semantic_policy p ON p.project_id=pr.id
 LEFT JOIN ai.available_model m ON m.capability='text.embed' AND m.name=pr.default_models->>'text.embed'
 LEFT JOIN ai.budget b ON b.scope_id=pr.owner_scope_id AND b.id=p.budget_id WHERE pr.id=?`, ep.Profile, project).Scan(&out).Error
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func runRAGEval(ctx context.Context, db *gorm.DB, project, actor, trial uuid.UUID, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	const maxBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBytes {
		return errors.New("evaluation manifest exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cases []aigateway.EvidenceCase
	if err := decoder.Decode(&cases); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid trailing evaluation data")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	runtime, err := aigateway.New(db, cfg)
	if err != nil {
		return err
	}
	out, trialErr := runtime.EvaluateEvidence(ctx, project, actor, trial, cases)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return err
	}
	return trialErr
}
