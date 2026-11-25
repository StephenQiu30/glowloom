export type AgentErrorCode =
  | 'invalid_session_update'
  | 'invalid_cursor'
  | 'invalid_event_cursor'
  | 'invalid_snapshot_watermark'
  | 'cursor_snapshot_mismatch'
  | 'invalid_service_response'
  | 'session_not_found'
  | 'submission_not_found'
  | 'message_not_found'
  | 'run_not_found'
  | 'request_identity_conflict'
  | 'session_has_active_run'
  | 'execution_revision_conflict'
  | 'retry_input_changed'
  | 'decision_identity_conflict'
  | 'decision_scope_conflict'
  | 'authorization_revision_gap'
  | 'snapshot_required'
  | 'cursor_expired'
  | 'snapshot_state_exceeds_limit'
  | 'quoted_message_exceeds_limit'
  | 'shutting_down';

export class AgentError extends Error {
  constructor(readonly code: AgentErrorCode) {
    super(code);
    this.name = 'AgentError';
  }
}
