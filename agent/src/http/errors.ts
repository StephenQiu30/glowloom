import { Catch, HttpException } from '@nestjs/common';
import type { ArgumentsHost, ExceptionFilter } from '@nestjs/common';
import type { FastifyReply, FastifyRequest } from 'fastify';
import { AgentError } from '../sessions/errors.js';
import type { AgentErrorCode } from '../sessions/errors.js';
import { InvalidRequest } from './validation.js';

const statusByErrorCode: Record<AgentErrorCode, number> = {
  invalid_cursor: 400,
  invalid_session_update: 400,
  invalid_event_cursor: 400,
  invalid_snapshot_watermark: 400,
  cursor_snapshot_mismatch: 400,
  invalid_service_response: 500,
  session_not_found: 404,
  submission_not_found: 404,
  message_not_found: 404,
  run_not_found: 404,
  request_identity_conflict: 409,
  session_has_active_run: 409,
  execution_revision_conflict: 409,
  retry_input_changed: 409,
  decision_identity_conflict: 409,
  decision_scope_conflict: 409,
  authorization_revision_gap: 409,
  snapshot_required: 409,
  cursor_expired: 410,
  snapshot_state_exceeds_limit: 413,
  quoted_message_exceeds_limit: 413,
  shutting_down: 503,
};

function httpStatus(error: unknown): number {
  if (error instanceof AgentError) return statusByErrorCode[error.code];
  if (error instanceof HttpException) return error.getStatus();
  if (
    error !== null &&
    typeof error === 'object' &&
    'statusCode' in error &&
    typeof error.statusCode === 'number' &&
    error.statusCode >= 400 &&
    error.statusCode < 600
  ) {
    return error.statusCode;
  }
  return 500;
}

export function sendHttpError(
  error: unknown,
  request: FastifyRequest,
  reply: FastifyReply,
) {
  if (reply.sent) return;
  const status = httpStatus(error);
  let code = status === 500 ? 'internal_error' : 'request_failed';
  if (error instanceof AgentError) code = error.code;
  if (error instanceof InvalidRequest) code = 'invalid_request';
  if (status >= 500) request.log.error({ code }, 'Agent request failed');
  return reply.code(status).send({ code, request_id: request.id });
}

@Catch()
export class AgentExceptionFilter implements ExceptionFilter {
  catch(error: unknown, host: ArgumentsHost) {
    const context = host.switchToHttp();
    sendHttpError(
      error,
      context.getRequest<FastifyRequest>(),
      context.getResponse<FastifyReply>(),
    );
  }
}
