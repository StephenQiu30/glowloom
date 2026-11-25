import { BusinessError } from '../business/client.js';

export interface ActionableToolError {
  readonly code: string;
  readonly message: string;
  readonly suggested_action: string;
  readonly retryable: boolean;
}

/**
 * Formats business/runtime errors into structured self-correcting feedback for the model,
 * so the model knows whether to re-read state, narrow scope, or stop and ask the user.
 */
export function formatActionableToolError(
  toolName: string,
  error: unknown,
): ActionableToolError {
  if (error instanceof BusinessError) {
    switch (error.code) {
      case 'revision_conflict':
      case 'conflict':
      case 'stale_revision':
        return {
          code: error.code,
          message: `Tool ${toolName} encountered a version conflict.`,
          suggested_action:
            'Re-read the latest resource revision before retrying; do not guess or overwrite newer edits.',
          retryable: true,
        };
      case 'conversation_reference_unavailable':
      case 'source_unavailable':
      case 'not_found':
        return {
          code: error.code,
          message: `Tool ${toolName} could not access the requested resource.`,
          suggested_action:
            'Inform the user that the referenced asset or target is unavailable and ask them to re-select a valid source.',
          retryable: false,
        };
      case 'budget_exceeded':
      case 'insufficient_credits':
      case 'agent_authorization_blocked':
        return {
          code: error.code,
          message: `Tool ${toolName} was blocked by authorization or budget policy.`,
          suggested_action:
            'Stop further paid or write actions and explain the authorization or budget constraint to the user.',
          retryable: false,
        };
      default:
        return {
          code: error.code,
          message: `Business tool ${toolName} failed with status ${error.status} (${error.code}).`,
          suggested_action:
            error.status >= 500
              ? 'Do not blindly resubmit paid side effects; inspect original receipts before retrying.'
              : 'Verify tool arguments against the schema and current project snapshot before retrying.',
          retryable: error.status >= 500,
        };
    }
  }
  const raw = error instanceof Error ? error.message : 'Tool execution failed';
  return {
    code: 'tool_execution_error',
    message: `Tool ${toolName} failed: ${raw}`,
    suggested_action:
      'Review the error message and current state; ask the user if required input is missing or ambiguous.',
    retryable: false,
  };
}
