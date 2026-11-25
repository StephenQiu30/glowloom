export type AgentErrorCode =
  | 'invalid_request'
  | 'session_not_found'
  | 'run_not_found'
  | 'submission_not_found'
  | 'message_not_found'
  | 'revision_conflict'
  | 'idempotency_conflict'
  | 'run_already_active'
  | 'invalid_cursor'
  | 'shutting_down'
  | 'internal_error';

export class AgentError extends Error {
  readonly statusCode: number;

  constructor(
    readonly code: AgentErrorCode,
    message: string,
    statusCode?: number,
    options?: { cause?: unknown },
  ) {
    super(message, options);
    this.name = 'AgentError';
    this.statusCode = statusCode ?? defaultStatusForCode(code);
  }
}

function defaultStatusForCode(code: AgentErrorCode): number {
  switch (code) {
    case 'invalid_request':
    case 'invalid_cursor':
      return 422;
    case 'session_not_found':
    case 'run_not_found':
    case 'submission_not_found':
    case 'message_not_found':
      return 404;
    case 'revision_conflict':
    case 'idempotency_conflict':
    case 'run_already_active':
      return 409;
    case 'shutting_down':
      return 503;
    default:
      return 500;
  }
}
