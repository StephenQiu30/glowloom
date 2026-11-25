import type { RunStatus } from '../database/schema.js';

export interface Identity {
  actorId: string;
  orgId?: string;
  scopeId?: string;
  projectId: string;
}

export interface RunEvent {
  id: string;
  seq: number;
  runId: string;
  event: string;
  data: Record<string, unknown>;
}

export interface SessionView {
  id: string;
  actor_id: string;
  org_id: string;
  scope_id: string;
  project_id: string;
  canvas_id?: string;
  title: string;
  execution_revision: number;
  pinned: boolean;
  archived_at?: string;
  created_at: string;
  last_input_at?: string;
  run?: RunView;
}

export interface PublicMessageView {
  message_id: string;
  run_id: string;
  conversation_id: number;
  sequence: string;
  part_index: number;
  kind: string;
  content: string;
}

export interface RunView {
  root_run_id: string;
  session_id: string;
  submission_id: string;
  goal: string;
  manifest: Record<string, unknown>;
  execution_available: boolean;
  state: {
    status: RunStatus;
    execution_revision: number;
    workflow_run_id?: string;
    error_code?: string;
  };
  snapshot_cursor: string;
  messages_page: {
    items: PublicMessageView[];
    through: string;
  };
  events: Array<{
    id: string;
    event: string;
    data: Record<string, unknown>;
  }>;
}

export interface CreateSessionInput {
  identity: Identity;
  requestId: string;
  title: string;
  canvasId?: string;
}

export interface UpdateSessionInput {
  identity: Identity;
  sessionId: string;
  title?: string;
  pinned?: boolean;
  archived?: boolean;
}

export interface StartRunInput {
  identity: Identity;
  sessionId: string;
  requestId: string;
  goal: string;
  manifest: Record<string, unknown>;
  input?: Record<string, unknown>;
  retryOfRunId?: string;
  expectedExecutionRevision: number;
  budgetMicros?: number;
  traceparent?: string;
}

export interface ApplyDecisionInput {
  identity: Identity;
  runId: string;
  decisionId: string;
  authorizationRevision: number;
  action: string;
  payload: Record<string, unknown>;
}

export interface AgentApplication {
  ready(): Promise<boolean>;
  createSession(input: CreateSessionInput): Promise<SessionView>;
  listSessions(
    identity: Identity,
    limit?: number,
    cursor?: string,
  ): Promise<{ items: SessionView[]; next_cursor?: string }>;
  getSession(identity: Identity, sessionId: string): Promise<SessionView>;
  updateSession(input: UpdateSessionInput): Promise<SessionView>;
  startRun(input: StartRunInput): Promise<Record<string, unknown>>;
  listRuns(
    identity: Identity,
    sessionId: string,
    limit?: number,
    cursor?: string,
  ): Promise<{ items: Array<Record<string, unknown>>; next_cursor?: string }>;
  getRun(identity: Identity, runId: string): Promise<RunView>;
  getCurrentSubmission(
    identity: Identity,
    sessionId: string,
  ): Promise<Record<string, unknown>>;
  getSubmission(
    identity: Identity,
    sessionId: string,
    requestId: string,
  ): Promise<Record<string, unknown>>;
  listMessages(
    identity: Identity,
    runId: string,
    limit?: number,
    before?: string,
    through?: string,
  ): Promise<{ items: PublicMessageView[]; next_cursor?: string; through: string }>;
  getMessage(
    identity: Identity,
    runId: string,
    messageId: string,
  ): Promise<PublicMessageView>;
  applyDecision(input: ApplyDecisionInput): Promise<Record<string, unknown>>;
  listRunEventsAfter(
    identity: Identity,
    runId: string,
    lastEventId?: string,
  ): Promise<{ events: RunEvent[]; terminal: boolean }>;
  subscribeRunEvents(
    runId: string,
    handler: (event: RunEvent) => void,
  ): () => void;
  startRecovery(): void;
  stopRecovery(): Promise<void>;
}
