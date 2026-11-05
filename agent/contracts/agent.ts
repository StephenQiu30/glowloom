import { Type, type Static } from 'typebox';

const Id = Type.String({ format: 'uuid' });
const Hash = Type.String({ pattern: '^[a-f0-9]{64}$' });
export const ScopeSchema = Type.Object({
  actor_id: Id,
  org_id: Id,
  scope_id: Id,
  project_id: Id,
  canvas_id: Type.Optional(Id),
});
export type Scope = Static<typeof ScopeSchema>;
const fields = ScopeSchema.properties;
export const IdentityQuery = Type.Object({ actor_id: Id, project_id: Id });
export type Identity = Static<typeof IdentityQuery>;
export const SessionParams = Type.Object({ sid: Id });
export const RunParams = Type.Object({ rid: Id });
export const HistoryQuery = Type.Object({
  ...IdentityQuery.properties,
  limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 50, default: 20 })),
  cursor: Type.Optional(Type.String({ maxLength: 2048 })),
});
export type HistoryInput = Static<typeof HistoryQuery>;
export const MessageQuery = Type.Object({
  ...IdentityQuery.properties,
  limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 100, default: 50 })),
  before: Type.Optional(Type.String({ maxLength: 2048 })),
  through: Type.String({ pattern: '^[0-9]+:[0-9]+$', maxLength: 64 }),
});
export type MessageInput = Static<typeof MessageQuery>;
export const SubmissionParams = Type.Object({ sid: Id, requestId: Id });
export const MessageParams = Type.Object({
  rid: Id,
  messageId: Type.String({ minLength: 1, maxLength: 256 }),
});
export const AuthorizedInputSchema = Type.Object(
  {
    schema_version: Type.Literal(1),
    references_unavailable: Type.Optional(Type.Boolean()),
    draft: Type.Record(Type.String(), Type.Unknown()),
    input_hash: Hash,
    plain_text: Type.String({ maxLength: 1048576 }),
    prompt: Type.String({ maxLength: 1048576 }),
    media_pins: Type.Array(
      Type.Object({
        sourceId: Id,
        sourceVersion: Type.String({ minLength: 1, maxLength: 256 }),
        mediaKind: Type.Union([
          Type.Literal('image'),
          Type.Literal('video'),
          Type.Literal('audio'),
        ]),
        bindingId: Type.Optional(Type.String()),
      }),
      { maxItems: 4 },
    ),
    quote: Type.Optional(
      Type.Object({
        run_id: Id,
        message_id: Type.String({ minLength: 1, maxLength: 256 }),
        text: Type.String({ maxLength: 65536 }),
        content_hash: Hash,
      }),
    ),
  },
  { additionalProperties: false },
);
export type AuthorizedInput = Static<typeof AuthorizedInputSchema>;
export const CreateSessionSchema = Type.Object(
  {
    ...fields,
    request_id: Id,
    content_hash: Hash,
    title: Type.String({ minLength: 1, maxLength: 240 }),
  },
  { additionalProperties: false },
);
export type CreateSessionInput = Static<typeof CreateSessionSchema>;
export const CreateRunSchema = Type.Object(
  {
    ...fields,
    request_id: Id,
    content_hash: Hash,
    expected_execution_revision: Type.Integer({
      minimum: 0,
      maximum: Number.MAX_SAFE_INTEGER,
    }),
    goal: Type.String({ minLength: 1, maxLength: 1048576 }),
    input: AuthorizedInputSchema,
    retry_of_run_id: Type.Optional(Id),
    manifest: Type.Record(Type.String(), Type.Unknown()),
  },
  { additionalProperties: false },
);
export type CreateRunInput = Static<typeof CreateRunSchema>;
export const ControlSchema = Type.Object(
  {
    ...fields,
    decision_id: Id,
    root_run_id: Id,
    session_id: Id,
    authorization_revision: Type.Integer({
      minimum: 1,
      maximum: Number.MAX_SAFE_INTEGER,
    }),
    action: Type.Union([
      Type.Literal('approve'),
      Type.Literal('reject'),
      Type.Literal('pause'),
      Type.Literal('resume'),
      Type.Literal('stop'),
    ]),
    content_hash: Hash,
  },
  { additionalProperties: false },
);
export type ControlInput = Static<typeof ControlSchema>;
export const operationIds = {
  createSession: 'createAgentSession',
  listSessions: 'listAgentSessions',
  readSession: 'getAgentSession',
  createRun: 'createAgentRun',
  readRun: 'getAgentRun',
  command: 'applyAgentDecision',
  events: 'subscribeAgentEvents',
  listRuns: 'listAgentRuns',
  readSubmission: 'getAgentSubmission',
  currentSubmission: 'getCurrentAgentSubmission',
  listMessages: 'listAgentMessages',
  readMessage: 'getAgentMessage',
} as const;

const JsonMap = Type.Record(Type.String(), Type.Unknown());
export const PlanItemSchema = Type.Object({
  id: Type.String(),
  title: Type.String(),
  status: Type.String(),
  depends_on: Type.Optional(Type.Array(Type.String())),
  artifacts: Type.Optional(Type.Array(Type.String())),
});
export const ProposalSchema = Type.Object({
  proposal_id: Id,
  candidate_hash: Type.String(),
  authorization_revision: Type.Integer(),
  candidate: JsonMap,
  canvas_revision: Type.Optional(Type.Integer()),
  price_version: Type.Optional(Type.String()),
  quote_micros: Type.Optional(Type.Integer()),
  budget_micros: Type.Optional(Type.Integer()),
});
export const AgentStateSchema = Type.Object({
  status: Type.String(),
  execution_revision: Type.Integer(),
  authorization_revision: Type.Optional(Type.Integer()),
  root_run_id: Type.Optional(Id),
  segment_id: Type.Optional(Type.String()),
  model_steps: Type.Optional(Type.Integer()),
  tool_steps: Type.Optional(Type.Integer()),
  goal: Type.Optional(Type.String()),
  plan: Type.Optional(Type.Array(PlanItemSchema)),
  delegations: Type.Optional(
    Type.Array(
      Type.Object({
        task_id: Type.String(),
        conversation_id: Type.Integer(),
        role: Type.String(),
        status: Type.String(),
        source_refs: Type.Optional(Type.Array(Type.String())),
        adopted: Type.Optional(Type.Boolean()),
        model_budget: Type.Optional(Type.Integer()),
        tool_budget: Type.Optional(Type.Integer()),
        model_used: Type.Optional(Type.Integer()),
        tool_used: Type.Optional(Type.Integer()),
      }),
    ),
  ),
  adopted: Type.Optional(Type.Array(JsonMap)),
  pending_proposal: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  proposal: Type.Optional(ProposalSchema),
  reconciliation: Type.Optional(JsonMap),
  failure: Type.Optional(JsonMap),
});
export const PublicMessageSchema = Type.Object({
  message_id: Type.String(),
  run_id: Id,
  conversation_id: Type.Integer(),
  sequence: Type.String(),
  part_index: Type.Integer({ minimum: 0 }),
  kind: Type.String(),
  content: Type.String(),
});
export const MessagePageSchema = Type.Object({
  items: Type.Array(PublicMessageSchema),
  next_cursor: Type.Optional(Type.String()),
  through: Type.String(),
});
export const RunSnapshotSchema = Type.Object({
  execution_available: Type.Boolean(),
  history_incomplete: Type.Optional(Type.Boolean()),
  messages_page: MessagePageSchema,
  root_run_id: Id,
  session_id: Id,
  submission_id: Type.String(),
  goal: Type.String(),
  input: Type.Optional(AuthorizedInputSchema),
  retry_of_run_id: Type.Optional(Id),
  manifest: JsonMap,
  state: AgentStateSchema,
  snapshot_cursor: Type.String(),
  events: Type.Array(
    Type.Object({ event_type: Type.String(), payload: JsonMap }),
  ),
});
export const SessionViewSchema = Type.Object({
  ...fields,
  id: Id,
  title: Type.String(),
  execution_revision: Type.Integer(),
  created_at: Type.Optional(Type.String({ format: 'date-time' })),
  run: Type.Optional(RunSnapshotSchema),
});
export const AcceptedRunSchema = Type.Object({
  root_run_id: Id,
  session_id: Id,
  submission_id: Type.String(),
  status: Type.String(),
  execution_revision: Type.Integer(),
});
export const ControlReceiptSchema = Type.Object({
  decision_id: Id,
  authorization_revision: Type.Integer(),
  status: Type.String(),
  execution_revision: Type.Integer(),
  receipt_id: Id,
});

export const SessionListSchema = Type.Object({
  items: Type.Array(SessionViewSchema),
  next_cursor: Type.Optional(Type.String()),
});
export const RunSummarySchema = Type.Object({
  root_run_id: Id,
  session_id: Id,
  accepted_revision: Type.Integer(),
  created_at: Type.String({ format: 'date-time' }),
  status: Type.String(),
  input_excerpt: Type.String({ maxLength: 480 }),
  retry_of_run_id: Type.Optional(Id),
});
export const RunPageSchema = Type.Object({
  items: Type.Array(RunSummarySchema),
  next_cursor: Type.Optional(Type.String()),
});
export const CurrentSubmissionSchema = Type.Object({
  latest_accepted: Type.Optional(AcceptedRunSchema),
  active_run: Type.Optional(RunSummarySchema),
  unresolved_run_ids: Type.Optional(Type.Array(Id)),
});
