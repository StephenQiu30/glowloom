import type { JsonObject } from '@earendil-works/pi-durable';
import { sql } from 'drizzle-orm';
import {
  bigint,
  boolean,
  customType,
  integer,
  jsonb,
  pgSchema,
  text,
  timestamp,
  uuid,
} from 'drizzle-orm/pg-core';
import type { FrozenManifest } from '../sessions/manifest.js';
import type { AuthorizedInput, ControlInput } from '../sessions/types.js';

// Read/write mappings only. backend/db/schema.sql owns structure, constraints,
// defaults and grants; this service does not load any schema/migration executor.
const agent = pgSchema('agent');
const sequence = (name: string) => bigint(name, { mode: 'number' });
const instant = (name: string) => timestamp(name, { withTimezone: true });
const decimalInteger = customType<{ data: string; driverData: string }>({
  dataType: () => 'bigint',
  fromDriver: (value) => String(value),
  toDriver: (value) => value,
});

export const runtimeSessions = agent.table('runtime_sessions', {
  session_id: text('session_id').primaryKey(),
  owner_id: text('owner_id').notNull(),
  owner_epoch: decimalInteger('owner_epoch').notNull(),
  lease_until: instant('lease_until').notNull(),
  last_id: sequence('last_id').notNull().default(1),
  commit_seq: sequence('commit_seq').notNull().default(0),
});

export const runtimeRecords = agent.table('runtime_records', {
  session_id: text('session_id').notNull(),
  id: sequence('id').notNull(),
  record_type: text('record_type').notNull(),
  body: text('body').notNull(),
  commit_seq: sequence('commit_seq').notNull(),
  conversation_id: sequence('conversation_id'),
  owner_conversation_id: sequence('owner_conversation_id'),
  owner_task_id: sequence('owner_task_id'),
  parent_id: sequence('parent_id'),
  parent_at: sequence('parent_at'),
  head_id: sequence('head_id'),
  kind_key: text('kind_key'),
  status: text('status'),
  background: boolean('background'),
  abort_requested: boolean('abort_requested'),
  request_key: text('request_key'),
  scope_key: text('scope_key'),
  address_key: text('address_key'),
  created_at_seq: sequence('created_at_seq'),
  retired_at_seq: sequence('retired_at_seq'),
});

export const runtimeDocumentVersions = agent.table(
  'runtime_document_versions',
  {
    session_id: text('session_id').notNull(),
    document_id: sequence('document_id').notNull(),
    commit_seq: sequence('commit_seq').notNull(),
    version: integer('version').notNull(),
    content: text('content').notNull(),
    deltas_since_base: integer('deltas_since_base').notNull(),
  },
);

export const runtimeEvents = agent.table('runtime_events', {
  session_id: text('session_id').notNull(),
  commit_seq: sequence('commit_seq').notNull(),
  event_ordinal: integer('event_ordinal').notNull(),
  event_type: text('event_type').notNull(),
  payload: jsonb('payload').$type<JsonObject>().notNull(),
});

export const serviceSessions = agent.table('service_sessions', {
  id: uuid('id').primaryKey(),
  actor_id: uuid('actor_id').notNull(),
  org_id: uuid('org_id').notNull(),
  scope_id: uuid('scope_id').notNull(),
  project_id: uuid('project_id').notNull(),
  canvas_id: uuid('canvas_id'),
  title: text('title').notNull(),
  request_id: uuid('request_id').notNull(),
  content_hash: text('content_hash').notNull(),
  execution_revision: decimalInteger('execution_revision')
    .notNull()
    .default('0'),
  created_at: instant('created_at')
    .notNull()
    .default(sql`clock_timestamp()`),
  last_input_at: instant('last_input_at'),
  pinned: boolean('pinned').notNull().default(false),
  archived_at: instant('archived_at'),
});

export const serviceRuns = agent.table('service_runs', {
  id: uuid('id').primaryKey(),
  session_id: uuid('session_id').notNull(),
  request_id: uuid('request_id').notNull(),
  content_hash: text('content_hash').notNull(),
  goal: text('goal').notNull(),
  manifest: jsonb('manifest').$type<FrozenManifest>().notNull(),
  submission_id: uuid('submission_id').notNull(),
  accepted_revision: decimalInteger('accepted_revision').notNull(),
  created_at: instant('created_at')
    .notNull()
    .default(sql`clock_timestamp()`),
  input: jsonb('input').$type<AuthorizedInput>(),
  retry_of_run_id: uuid('retry_of_run_id'),
});

export const serviceControlInbox = agent.table('service_control_inbox', {
  decision_id: uuid('decision_id').primaryKey(),
  root_run_id: uuid('root_run_id').notNull(),
  authorization_revision: decimalInteger('authorization_revision').notNull(),
  content_hash: text('content_hash').notNull(),
  input: jsonb('input').$type<ControlInput>().notNull(),
  receipt: jsonb('receipt').$type<JsonObject>(),
  created_at: instant('created_at')
    .notNull()
    .default(sql`clock_timestamp()`),
});

export const publicMessages = agent.table('public_messages', {
  session_id: text('session_id').notNull(),
  run_id: uuid('run_id').notNull(),
  commit_seq: sequence('commit_seq').notNull(),
  event_ordinal: integer('event_ordinal').notNull(),
  part_index: integer('part_index').notNull(),
  message_id: uuid('message_id').notNull(),
  conversation_id: sequence('conversation_id').notNull(),
  sequence: text('sequence').notNull(),
  kind: text('kind').notNull(),
  content: text('content').notNull(),
});

export const recoveryBackoff = agent.table('recovery_backoff', {
  kind: text('kind').notNull(),
  subject_id: uuid('subject_id').notNull(),
  root_run_id: uuid('root_run_id').notNull(),
  attempts: integer('attempts').notNull().default(0),
  code: text('code'),
  next_attempt_at: instant('next_attempt_at')
    .notNull()
    .default(sql`clock_timestamp()`),
  parked_at: instant('parked_at'),
});
