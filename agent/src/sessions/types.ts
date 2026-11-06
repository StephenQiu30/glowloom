export interface Scope {
  actor_id: string;
  org_id: string;
  scope_id: string;
  project_id: string;
  canvas_id?: string;
}

export interface Identity {
  actor_id: string;
  project_id: string;
}

export interface HistoryInput extends Identity {
  limit?: number;
  cursor?: string;
}

export interface MessageInput extends Identity {
  limit?: number;
  before?: string;
  through: string;
}

export interface AuthorizedInput {
  schema_version: 1;
  references_unavailable?: boolean;
  draft: Record<string, unknown>;
  input_hash: string;
  plain_text: string;
  prompt: string;
  media_pins: Array<{
    sourceId: string;
    sourceVersion: string;
    mediaKind: 'image' | 'video' | 'audio';
    bindingId?: string;
  }>;
  quote?: {
    run_id: string;
    message_id: string;
    text: string;
    content_hash: string;
  };
}

export interface CreateSessionInput extends Scope {
  request_id: string;
  content_hash: string;
  title: string;
}

export interface UpdateSessionInput extends Scope {
  action: 'pin' | 'unpin' | 'rename' | 'delete';
  title?: string;
}

export interface CreateRunInput extends Scope {
  request_id: string;
  content_hash: string;
  expected_execution_revision: number;
  goal: string;
  input: AuthorizedInput;
  retry_of_run_id?: string;
  manifest: Record<string, unknown>;
}

export interface ControlInput extends Scope {
  decision_id: string;
  root_run_id: string;
  session_id: string;
  authorization_revision: number;
  action: 'approve' | 'reject' | 'pause' | 'resume' | 'stop';
  content_hash: string;
}

export interface PublicMessage {
  message_id: string;
  run_id: string;
  conversation_id: number;
  sequence: string;
  part_index: number;
  kind: string;
  content: string;
}

export interface MessagePage {
  items: PublicMessage[];
  next_cursor?: string;
  through: string;
}

export interface RunSummary {
  root_run_id: string;
  session_id: string;
  accepted_revision: number;
  created_at: string;
  status: string;
  input_excerpt: string;
  retry_of_run_id?: string;
}

export interface RunPage {
  items: RunSummary[];
  next_cursor?: string;
}

export interface RuntimeEvent {
  cursor: string;
  type: string;
  payload: unknown;
}

export interface AgentApplication {
  ready(): Promise<boolean>;
  createSession(input: CreateSessionInput): Promise<unknown>;
  updateSession(id: string, input: UpdateSessionInput): Promise<unknown>;
  listSessions(identity: HistoryInput): Promise<unknown>;
  readSession(id: string, identity: Identity): Promise<unknown>;
  createRun(sessionId: string, input: CreateRunInput): Promise<unknown>;
  readRun(id: string, identity: Identity): Promise<unknown>;
  command(id: string, input: ControlInput): Promise<unknown>;
  events(
    id: string,
    identity: Identity,
    after: string | undefined,
  ): Promise<RuntimeEvent[]>;
  stopAccepting(): void;
  close(): Promise<void>;
  listRuns(sessionId: string, query: HistoryInput): Promise<unknown>;
  readSubmission(
    sessionId: string,
    requestId: string,
    identity: Identity,
  ): Promise<unknown>;
  currentSubmission(sessionId: string, identity: Identity): Promise<unknown>;
  listMessages(runId: string, query: MessageInput): Promise<unknown>;
  readPublicMessage(
    runId: string,
    messageId: string,
    identity: Identity,
  ): Promise<unknown>;
}
