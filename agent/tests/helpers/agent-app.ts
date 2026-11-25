import type { AgentApplication } from '../../src/sessions/types.js';

const http: typeof import('../../src/transport/app.js') = await import(
  new URL('../../dist/src/transport/app.js', import.meta.url).href
);
const openApi: typeof import('../../src/transport/openapi.js') = await import(
  new URL('../../dist/src/transport/openapi.js', import.meta.url).href
);
const errors: typeof import('../../src/sessions/session-errors.js') =
  await import(
    new URL('../../dist/src/sessions/session-errors.js', import.meta.url).href
  );

export const { buildApp } = http;
export const { createOpenApi } = openApi;
export const { AgentError } = errors;

export function agentFixture(
  overrides: Partial<AgentApplication> = {},
): AgentApplication {
  const unused = async () => {
    throw new Error('Unexpected Agent application call');
  };
  return {
    ready: async () => true,
    createSession: unused,
    updateSession: unused,
    listSessions: unused,
    readSession: unused,
    createRun: unused,
    readRun: unused,
    command: unused,
    events: unused,
    stopAccepting: () => {},
    close: async () => {},
    listRuns: unused,
    readSubmission: unused,
    currentSubmission: unused,
    listMessages: unused,
    readPublicMessage: unused,
    ...overrides,
  };
}
