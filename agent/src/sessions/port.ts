import type {
  CreateRunInput,
  CreateSessionInput,
  UpdateSessionInput,
  ControlInput,
  HistoryInput,
  Identity,
  MessageInput,
} from '../../contracts/agent.js';

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
