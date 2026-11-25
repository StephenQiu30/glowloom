import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import { AgentError } from '../sessions/errors.js';
import type {
  AgentApplication,
  Identity,
  RunEvent,
} from '../sessions/types.js';
import { verifyServiceIdentity } from './auth.js';

const maxMessageBytes = 8 * 1024 * 1024;

interface ProtoIdentity {
  actor_id?: string;
  org_id?: string;
  scope_id?: string;
  project_id?: string;
}

interface PayloadResponse {
  status_code: number;
  payload_json: Buffer;
}

interface RunEventFrame {
  id: string;
  event: string;
  data_json: Buffer;
}

export interface AgentGrpcServer {
  address: string;
  close(): Promise<void>;
}

function resolveProtoPath(): string {
  const currentDir = path.dirname(fileURLToPath(import.meta.url));
  const candidates = [
    path.resolve(currentDir, '../../../proto/runtime.proto'),
    path.resolve(currentDir, '../../proto/runtime.proto'),
    path.resolve(
      currentDir,
      '../../../../backend/api/proto/glowloom/agent/runtime.proto',
    ),
    path.resolve(
      currentDir,
      '../../../backend/api/proto/glowloom/agent/runtime.proto',
    ),
  ];
  const found = candidates.find((candidate) => existsSync(candidate));
  if (!found) {
    throw new Error('AGENT_PROTO_NOT_FOUND');
  }
  return found;
}

function verifyMetadata(
  metadata: grpc.Metadata,
  expectedAuth: Buffer,
): Partial<grpc.StatusObject> | null {
  const authValues = metadata.get('authorization');
  const rawAuth = typeof authValues[0] === 'string' ? authValues[0] : undefined;
  const audValues = metadata.get('x-agent-audience');
  const aud = typeof audValues[0] === 'string' ? audValues[0] : undefined;
  const failure = verifyServiceIdentity(rawAuth, aud, expectedAuth);
  if (failure === 'unauthorized') {
    return { code: grpc.status.UNAUTHENTICATED, details: 'UNAUTHORIZED' };
  }
  if (failure === 'invalid_audience') {
    return {
      code: grpc.status.PERMISSION_DENIED,
      details: 'INVALID_AUDIENCE',
    };
  }
  return null;
}

function extractTraceparent(metadata: grpc.Metadata): string | undefined {
  const values = metadata.get('traceparent');
  return typeof values[0] === 'string' ? values[0] : undefined;
}

function parseIdentity(raw: ProtoIdentity | undefined): Identity {
  const actorId = raw?.actor_id?.trim() ?? '';
  const projectId = raw?.project_id?.trim() ?? '';
  if (!actorId || !projectId) {
    throw new AgentError(
      'invalid_request',
      'identity.actor_id and identity.project_id are required',
    );
  }
  return {
    actorId,
    projectId,
    ...(raw?.org_id?.trim() ? { orgId: raw.org_id.trim() } : {}),
    ...(raw?.scope_id?.trim() ? { scopeId: raw.scope_id.trim() } : {}),
  };
}

function parseJsonBuffer(
  buf: Buffer | undefined,
): Record<string, unknown> | undefined {
  if (!buf || buf.length === 0) return undefined;
  const parsed: unknown = JSON.parse(buf.toString('utf8'));
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return undefined;
  }
  return parsed as Record<string, unknown>;
}

function toPayloadResponse(statusCode: number, body: unknown): PayloadResponse {
  return {
    status_code: statusCode,
    payload_json: Buffer.from(JSON.stringify(body), 'utf8'),
  };
}

function handleUnaryError(
  error: unknown,
  callback: grpc.sendUnaryData<PayloadResponse>,
): void {
  if (error instanceof AgentError) {
    callback(
      null,
      toPayloadResponse(error.statusCode, {
        code: error.code,
        message: error.message,
      }),
    );
    return;
  }
  callback({
    code: grpc.status.INTERNAL,
    details: error instanceof Error ? error.message : 'INTERNAL_ERROR',
  });
}

function grpcStatusForError(error: unknown): Partial<grpc.StatusObject> {
  if (error instanceof AgentError) {
    switch (error.code) {
      case 'run_not_found':
      case 'session_not_found':
      case 'submission_not_found':
      case 'message_not_found':
        return { code: grpc.status.NOT_FOUND, details: error.code };
      case 'invalid_request':
      case 'invalid_cursor':
        return { code: grpc.status.INVALID_ARGUMENT, details: error.code };
      case 'shutting_down':
        return { code: grpc.status.UNAVAILABLE, details: error.code };
      default:
        return { code: grpc.status.FAILED_PRECONDITION, details: error.code };
    }
  }
  return {
    code: grpc.status.INTERNAL,
    details: error instanceof Error ? error.message : 'INTERNAL_ERROR',
  };
}

function writeEventFrame(
  call: grpc.ServerWritableStream<unknown, RunEventFrame>,
  event: RunEvent,
): void {
  call.write({
    id: event.id,
    event: event.event,
    data_json: Buffer.from(JSON.stringify(event.data), 'utf8'),
  });
}

export async function startAgentGrpcServer(options: {
  service: AgentApplication;
  address: string;
  serviceToken: string;
}): Promise<AgentGrpcServer> {
  const packageDefinition = protoLoader.loadSync(resolveProtoPath(), {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: false,
    oneofs: true,
  });
  const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as {
    glowloom: {
      agent: {
        AgentRuntime: grpc.ServiceClientConstructor & {
          service: grpc.ServiceDefinition;
        };
      };
    };
  };

  const expectedAuth = Buffer.from(`Bearer ${options.serviceToken}`);
  const server = new grpc.Server({
    'grpc.max_receive_message_length': maxMessageBytes,
    'grpc.max_send_message_length': maxMessageBytes,
  });

  function guardUnary<Req>(
    handler: (req: Req, metadata: grpc.Metadata) => Promise<PayloadResponse>,
  ): grpc.handleUnaryCall<Req, PayloadResponse> {
    return (call, callback) => {
      const authError = verifyMetadata(call.metadata, expectedAuth);
      if (authError) {
        callback(authError);
        return;
      }
      void handler(call.request, call.metadata)
        .then((res) => callback(null, res))
        .catch((err: unknown) => handleUnaryError(err, callback));
    };
  }

  server.addService(protoDescriptor.glowloom.agent.AgentRuntime.service, {
    CreateSession: guardUnary<{
      identity?: ProtoIdentity;
      request_id?: string;
      title?: string;
      canvas_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const session = await options.service.createSession({
        identity,
        requestId: req.request_id ?? '',
        title: req.title ?? 'Untitled',
        ...(req.canvas_id ? { canvasId: req.canvas_id } : {}),
      });
      return toPayloadResponse(201, session);
    }),

    ListSessions: guardUnary<{
      identity?: ProtoIdentity;
      limit?: number;
      cursor?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const page = await options.service.listSessions(
        identity,
        req.limit,
        req.cursor,
      );
      return toPayloadResponse(200, page);
    }),

    GetSession: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const session = await options.service.getSession(
        identity,
        req.session_id ?? '',
      );
      return toPayloadResponse(200, session);
    }),

    UpdateSession: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
      title?: string;
      pinned?: boolean;
      archived?: boolean;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const session = await options.service.updateSession({
        identity,
        sessionId: req.session_id ?? '',
        ...(req.title !== undefined ? { title: req.title } : {}),
        ...(req.pinned !== undefined ? { pinned: req.pinned } : {}),
        ...(req.archived !== undefined ? { archived: req.archived } : {}),
      });
      return toPayloadResponse(200, session);
    }),

    StartRun: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
      request_id?: string;
      goal?: string;
      manifest_json?: Buffer;
      input_json?: Buffer;
      retry_of_run_id?: string;
      expected_execution_revision?: string | number;
      budget_micros?: string | number;
      content_hash?: string;
    }>(async (req, metadata) => {
      const identity = parseIdentity(req.identity);
      const traceparent = extractTraceparent(metadata);
      const inputPayload = parseJsonBuffer(req.input_json);
      const accepted = await options.service.startRun({
        identity,
        sessionId: req.session_id ?? '',
        requestId: req.request_id ?? '',
        goal: req.goal ?? '',
        manifest: parseJsonBuffer(req.manifest_json) ?? {},
        ...(inputPayload ? { input: inputPayload } : {}),
        ...(req.content_hash ? { contentHash: req.content_hash } : {}),
        ...(req.retry_of_run_id ? { retryOfRunId: req.retry_of_run_id } : {}),
        expectedExecutionRevision: Number(req.expected_execution_revision ?? 0),
        ...(req.budget_micros !== undefined
          ? { budgetMicros: Number(req.budget_micros) }
          : {}),
        ...(traceparent ? { traceparent } : {}),
      });
      return toPayloadResponse(202, accepted);
    }),

    ListRuns: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
      limit?: number;
      cursor?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const page = await options.service.listRuns(
        identity,
        req.session_id ?? '',
        req.limit,
        req.cursor,
      );
      return toPayloadResponse(200, page);
    }),

    GetRun: guardUnary<{
      identity?: ProtoIdentity;
      run_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const run = await options.service.getRun(identity, req.run_id ?? '');
      return toPayloadResponse(200, run);
    }),

    GetCurrentSubmission: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const submission = await options.service.getCurrentSubmission(
        identity,
        req.session_id ?? '',
      );
      return toPayloadResponse(200, submission);
    }),

    GetSubmission: guardUnary<{
      identity?: ProtoIdentity;
      session_id?: string;
      request_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const submission = await options.service.getSubmission(
        identity,
        req.session_id ?? '',
        req.request_id ?? '',
      );
      return toPayloadResponse(200, submission);
    }),

    ListMessages: guardUnary<{
      identity?: ProtoIdentity;
      run_id?: string;
      limit?: number;
      before?: string;
      through?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const page = await options.service.listMessages(
        identity,
        req.run_id ?? '',
        req.limit,
        req.before,
        req.through,
      );
      return toPayloadResponse(200, page);
    }),

    GetMessage: guardUnary<{
      identity?: ProtoIdentity;
      run_id?: string;
      message_id?: string;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const message = await options.service.getMessage(
        identity,
        req.run_id ?? '',
        req.message_id ?? '',
      );
      return toPayloadResponse(200, message);
    }),

    ApplyDecision: guardUnary<{
      identity?: ProtoIdentity;
      run_id?: string;
      decision_id?: string;
      authorization_revision?: string | number;
      action?: string;
      payload_json?: Buffer;
    }>(async (req) => {
      const identity = parseIdentity(req.identity);
      const receipt = await options.service.applyDecision({
        identity,
        runId: req.run_id ?? '',
        decisionId: req.decision_id ?? '',
        authorizationRevision: Number(req.authorization_revision ?? 0),
        action: req.action ?? '',
        payload: parseJsonBuffer(req.payload_json) ?? {},
      });
      return toPayloadResponse(200, receipt);
    }),

    StreamRunEvents(
      call: grpc.ServerWritableStream<
        {
          identity?: ProtoIdentity;
          run_id?: string;
          last_event_id?: string;
        },
        RunEventFrame
      >,
    ) {
      const authError = verifyMetadata(call.metadata, expectedAuth);
      if (authError) {
        call.destroy(Object.assign(new Error(authError.details), authError));
        return;
      }

      void (async () => {
        let unsubscribe: (() => void) | undefined;
        try {
          const identity = parseIdentity(call.request.identity);
          const runId = call.request.run_id?.trim() ?? '';
          if (!runId) {
            throw new AgentError('invalid_request', 'run_id is required');
          }

          let lastSentSeq = call.request.last_event_id
            ? Number.parseInt(call.request.last_event_id, 10) || 0
            : 0;
          const buffered: RunEvent[] = [];
          let replaying = true;

          unsubscribe = options.service.subscribeRunEvents(runId, (ev) => {
            if (replaying) {
              buffered.push(ev);
              return;
            }
            if (ev.seq <= lastSentSeq) return;
            lastSentSeq = ev.seq;
            writeEventFrame(call, ev);
            if (
              ev.event === 'run.completed' ||
              ev.event === 'run.failed' ||
              ev.event === 'run.cancelled'
            ) {
              unsubscribe?.();
              call.end();
            }
          });

          call.on('cancelled', () => unsubscribe?.());
          call.on('close', () => unsubscribe?.());

          const snapshot = await options.service.listRunEventsAfter(
            identity,
            runId,
            call.request.last_event_id,
          );
          for (const ev of snapshot.events) {
            if (ev.seq > lastSentSeq) {
              lastSentSeq = ev.seq;
              writeEventFrame(call, ev);
            }
          }
          for (const ev of buffered) {
            if (ev.seq > lastSentSeq) {
              lastSentSeq = ev.seq;
              writeEventFrame(call, ev);
            }
          }
          replaying = false;

          if (snapshot.terminal) {
            unsubscribe?.();
            call.end();
          }
        } catch (error: unknown) {
          unsubscribe?.();
          const st = grpcStatusForError(error);
          call.destroy(Object.assign(new Error(st.details), st));
        }
      })();
    },

    CheckHealth(
      _call: grpc.ServerUnaryCall<Record<string, never>, { status: string }>,
      callback: grpc.sendUnaryData<{ status: string }>,
    ) {
      void options.service.ready().then((ok) => {
        callback(null, { status: ok ? 'SERVING' : 'NOT_SERVING' });
      });
    },
  });

  const boundPort = await new Promise<number>((resolve, reject) => {
    server.bindAsync(
      options.address,
      grpc.ServerCredentials.createInsecure(),
      (err, port) => {
        if (err) reject(err);
        else resolve(port);
      },
    );
  });

  const host =
    options.address.slice(0, options.address.lastIndexOf(':')) || '0.0.0.0';

  return {
    address: `${host}:${boundPort}`,
    close() {
      return new Promise<void>((resolve) => {
        server.tryShutdown(() => resolve());
      });
    },
  };
}
