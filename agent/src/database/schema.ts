import type { JsonObject } from '@earendil-works/pi-durable';
import type { Generated } from 'kysely';
import type {
  RuntimeSession,
  RuntimeRecord,
  RuntimeDocumentVersion,
  RuntimeEvent,
  ServiceSession,
  ServiceRun,
  ServiceControlInbox,
  PublicMessage,
  RecoveryBackoff,
} from './generated/client.js';
import type { FrozenManifest } from '../sessions/manifest.js';
import type { AuthorizedInput, ControlInput } from '../sessions/types.js';

// Builder types derive from the generated Prisma client. Neither mapping owns DDL.
// backend/db/schema.sql remains the only database structure source.
export interface Database {
  runtime_sessions: Omit<RuntimeSession, 'last_id' | 'commit_seq'> & {
    last_id: Generated<bigint>;
    commit_seq: Generated<bigint>;
  };
  runtime_records: RuntimeRecord;
  runtime_document_versions: RuntimeDocumentVersion;
  runtime_events: Omit<RuntimeEvent, 'payload'> & { payload: JsonObject };
  service_sessions: Omit<
    ServiceSession,
    'execution_revision' | 'created_at' | 'pinned'
  > & {
    execution_revision: Generated<bigint>;
    created_at: Generated<Date>;
    pinned: Generated<boolean>;
  };
  service_runs: Omit<ServiceRun, 'manifest' | 'input' | 'created_at'> & {
    manifest: FrozenManifest;
    input: AuthorizedInput | null;
    created_at: Generated<Date>;
  };
  service_control_inbox: Omit<
    ServiceControlInbox,
    'input' | 'receipt' | 'created_at'
  > & {
    input: ControlInput;
    receipt: JsonObject | null;
    created_at: Generated<Date>;
  };
  public_messages: PublicMessage;
  recovery_backoff: Omit<RecoveryBackoff, 'attempts' | 'next_attempt_at'> & {
    attempts: Generated<number>;
    next_attempt_at: Generated<Date>;
  };
}
