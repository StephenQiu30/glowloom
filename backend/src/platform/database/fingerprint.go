package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var requiredTables = []string{
	"workspaces", "projects", "operations", "outbox_events", "inbox_messages",
	"iam_users", "iam_memberships", "iam_roles", "iam_project_grants", "iam_sessions",
	"prj_brief_revisions", "prj_content_units", "prj_content_order_revisions", "prj_content_order_items",
	"nar_source_revisions", "nar_analysis_drafts", "nar_import_runs", "nar_analysis_runs", "nar_episode_breakdown_revisions", "nar_episode_candidates", "nar_episode_breakdown_manifests",
	"nar_narrative_revisions", "nar_scenes", "nar_narrative_nodes", "nar_production_element_mentions",
	"pk_entities", "pk_mention_resolutions", "pk_production_requirement_items", "pk_production_requirement_revisions",
	"sht_shots", "sht_shot_plan_revisions", "m06_agent_runs", "m06_proposal_items", "gen_plans", "gen_plan_items",
	"exec_generation_jobs", "exec_attempts", "media_artifacts", "media_artifact_locations", "media_candidates", "media_selection_decisions",
	"qa_evaluations", "qa_issues", "usage_reservations", "usage_entries", "review_packages", "review_decisions",
	"delivery_assembly_revisions", "delivery_builds", "delivery_snapshots", "gov_rights_declarations", "gov_policy_evaluations",
	"tpl_templates", "int_api_clients", "int_webhook_subscriptions", "ops_idempotency_records", "ops_operation_steps",
	"ops_task_attempts", "ops_search_projection_checkpoints", "audit_events",
}

var forbiddenCompatibilityTables = []string{
	"script_revisions", "script_analysis_drafts", "content_units", "narrative_units",
	"entities", "entity_mentions", "production_requirements",
}

var requiredTenantPolicies = []string{
	"projects", "iam_memberships", "iam_project_grants", "iam_sessions",
	"iam_service_identities", "audit_events", "tpl_templates", "int_api_clients",
	"int_webhook_subscriptions", "ops_idempotency_records", "ops_search_projection_checkpoints", "media_artifacts",
}

func VerifyCurrent(ctx context.Context, pool *pgxpool.Pool) error {
	for _, table := range requiredTables {
		var present bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&present); err != nil {
			return fmt.Errorf("check schema table %s: %w", table, err)
		}
		if !present {
			return fmt.Errorf("current schema is missing table %s", table)
		}
	}
	for _, table := range forbiddenCompatibilityTables {
		var present bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&present); err != nil {
			return fmt.Errorf("check retired schema table %s: %w", table, err)
		}
		if present {
			return fmt.Errorf("current schema contains retired compatibility table %s", table)
		}
	}
	for _, table := range requiredTenantPolicies {
		var present bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_policies
				WHERE schemaname = current_schema()
				  AND tablename = $1
				  AND policyname = 'tenant_isolation'
			)`, table).Scan(&present); err != nil {
			return fmt.Errorf("check tenant policy %s: %w", table, err)
		}
		if !present {
			return fmt.Errorf("current schema is missing tenant policy for %s", table)
		}
	}
	return nil
}

func EnsureEmpty(ctx context.Context, pool *pgxpool.Pool) error {
	var tableCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_tables
		WHERE schemaname = current_schema()
	`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect target schema: %w", err)
	}
	if tableCount != 0 {
		return fmt.Errorf("schema-init requires an empty database schema, found %d tables", tableCount)
	}
	return nil
}
