import type { ColumnType } from 'kysely';

export type RunStatus =
  | 'queued'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface ServiceSessionsTable {
  id: string;
  actor_id: string;
  org_id: string;
  scope_id: string;
  project_id: string;
  canvas_id: string | null;
  title: string;
  request_id: string;
  content_hash: string;
  execution_revision: ColumnType<
    string,
    string | number | bigint,
    string | number | bigint
  >;
  created_at: ColumnType<Date, Date | string | undefined, never>;
  last_input_at: ColumnType<
    Date | null,
    Date | string | null | undefined,
    Date | string | null
  >;
  pinned: ColumnType<boolean, boolean | undefined, boolean>;
  archived_at: ColumnType<
    Date | null,
    Date | string | null | undefined,
    Date | string | null
  >;
}

export interface ServiceRunsTable {
  id: string;
  session_id: string;
  request_id: string;
  content_hash: string;
  goal: string;
  manifest: Record<string, unknown>;
  skill_revisions: string[];
  submission_id: string;
  accepted_revision: ColumnType<
    string,
    string | number | bigint,
    string | number | bigint
  >;
  status: RunStatus;
  lease_owner: string | null;
  lease_until: ColumnType<
    Date | null,
    Date | string | null | undefined,
    Date | string | null
  >;
  workflow_run_id: string | null;
  error_code: string | null;
  max_model_requests: ColumnType<number, number | undefined, number>;
  used_model_requests: ColumnType<number, number | undefined, number>;
  max_tool_steps: ColumnType<number, number | undefined, number>;
  used_tool_steps: ColumnType<number, number | undefined, number>;
  input: Record<string, unknown> | null;
  retry_of_run_id: string | null;
  created_at: ColumnType<Date, Date | string | undefined, never>;
  updated_at: ColumnType<Date, Date | string | undefined, Date | string>;
  completed_at: ColumnType<
    Date | null,
    Date | string | null | undefined,
    Date | string | null
  >;
}

export interface ServiceRunEventsTable {
  run_id: string;
  seq: ColumnType<string, string | number | bigint, never>;
  event_type: string;
  payload: Record<string, unknown>;
  created_at: ColumnType<Date, Date | string | undefined, never>;
}

export interface ServicePublicMessagesTable {
  run_id: string;
  seq: ColumnType<string, string | number | bigint, never>;
  part_index: number;
  message_id: string;
  kind: string;
  content: string;
  created_at: ColumnType<Date, Date | string | undefined, never>;
}

export interface ServiceControlInboxTable {
  decision_id: string;
  root_run_id: string;
  authorization_revision: ColumnType<string, string | number | bigint, never>;
  content_hash: string;
  input: Record<string, unknown>;
  receipt: Record<string, unknown> | null;
  created_at: ColumnType<Date, Date | string | undefined, never>;
}

export interface RecoveryBackoffTable {
  kind: 'run' | 'control';
  subject_id: string;
  root_run_id: string;
  attempts: number;
  code: string | null;
  next_attempt_at: ColumnType<Date, Date | string | undefined, Date | string>;
  parked_at: ColumnType<
    Date | null,
    Date | string | null | undefined,
    Date | string | null
  >;
}

export interface AgentDatabase {
  'agent.skill_revisions': {
    digest: string;
    bundle: ColumnType<unknown, string, never>;
  };
  'agent.service_sessions': ServiceSessionsTable;
  'agent.service_runs': ServiceRunsTable;
  'agent.service_run_events': ServiceRunEventsTable;
  'agent.service_public_messages': ServicePublicMessagesTable;
  'agent.service_control_inbox': ServiceControlInboxTable;
  'agent.recovery_backoff': RecoveryBackoffTable;
}
