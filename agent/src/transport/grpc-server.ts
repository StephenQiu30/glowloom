import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { AgentError } from '../sessions/session-errors.js';
import type { AgentApplication, Identity } from '../sessions/types.js';
import { verifyServiceIdentity } from './auth.js';

const maxMessageBytes = 8 * 1024 * 1024;
const streamPollFallbackMs = 1000;

interface InvokeRequest {
  operation?: string;
  method?: string;
  path?: string;
  params?: Record<string, string>;
  query?: Record<string, string>;
  headers?: Record<string, string>;
  body_json?: Buffer;
}

interface InvokeResponse {
  status_code: number;
  content_type: string;
  body_json: Buffer;
}

interface StreamRunEventsRequest {
  run_id?: string;
  last_event_id?: string;
  query?: Record<string, string>;
}

interface RunEventFrame {
  id: string;
  event: string;
  data_json: Buffer;
}

interface HealthResponse {
  status: string;
}

export interface AgentGrpcServer {
  address: string;
  close(): Promise<void>;
}

function resolveProtoPath(): string {
  const currentDir = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(currentDir, '../../../proto/runtime.proto');
}

function verifyServiceMetadata(
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

function parseStreamIdentity(
  query: Record<string, string> | undefined,
): Identity | null {
  const actorId = query?.actor_id?.trim() ?? '';
  const projectId = query?.project_id?.trim() ?? '';
  if (!actorId || !projectId) return null;
  return { actor_id: actorId, project_id: projectId };
}

function grpcCodeForAgentError(error: AgentError): grpc.status {
  switch (error.code) {
    case 'run_not_found':
    case 'session_not_found':
    case 'submission_not_found':
    case 'message_not_found':
      return grpc.status.NOT_FOUND;
    case 'invalid_cursor':
    case 'invalid_event_cursor':
    case 'invalid_snapshot_watermark':
    case 'cursor_snapshot_mismatch':
    case 'invalid_session_update':
      return grpc.status.INVALID_ARGUMENT;
    case 'shutting_down':
      return grpc.status.UNAVAILABLE;
    default:
      return grpc.status.FAILED_PRECONDITION;
  }
}

export async function startAgentGrpcServer({
  app,
  service,
  address,
  serviceToken,
}: {
  app: NestFastifyApplication;
  service: AgentApplication;
  address: string;
  serviceToken: string;
}): Promise<AgentGrpcServer> {
  if (Buffer.byteLength(serviceToken) < 32) {
    throw new Error('AGENT_SERVICE_TOKEN_REQUIRED');
  }
  const expectedAuth = Buffer.from(`Bearer ${serviceToken}`);
  const packageDef = protoLoader.loadSync(resolveProtoPath(), {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true,
  });
  const loaded = grpc.loadPackageDefinition(packageDef) as unknown as {
    glowloom: {
      agent: {
        AgentRuntime: { service: grpc.ServiceDefinition };
      };
    };
  };

  const fastify = app.getHttpAdapter().getInstance();

  const server = new grpc.Server({
    'grpc.max_receive_message_length': maxMessageBytes,
    'grpc.max_send_message_length': maxMessageBytes,
  });

  server.addService(loaded.glowloom.agent.AgentRuntime.service, {
    CheckHealth: (
      _call: grpc.ServerUnaryCall<Record<string, never>, HealthResponse>,
      callback: grpc.sendUnaryData<HealthResponse>,
    ) => {
      callback(null, { status: 'ok' });
    },

    Invoke: async (
      call: grpc.ServerUnaryCall<InvokeRequest, InvokeResponse>,
      callback: grpc.sendUnaryData<InvokeResponse>,
    ) => {
      const authErr = verifyServiceMetadata(call.metadata, expectedAuth);
      if (authErr) {
        callback(authErr);
        return;
      }
      try {
        const req = call.request;
        const method = (req.method ?? 'GET').toUpperCase() as
          'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
        const qs = new URLSearchParams(req.query ?? {}).toString();
        const url = qs ? `${req.path ?? ''}?${qs}` : (req.path ?? '');
        const headers: Record<string, string> = {
          ...(req.headers ?? {}),
          authorization: `Bearer ${serviceToken}`,
          'x-agent-audience': 'agent',
        };
        const payload =
          req.body_json && req.body_json.length > 0 ? req.body_json : undefined;
        if (payload && !headers['content-type']) {
          headers['content-type'] = 'application/json';
        }
        const injected = await fastify.inject({
          method,
          url,
          headers,
          ...(payload ? { payload } : {}),
        });
        const contentType = String(
          injected.headers['content-type'] ?? 'application/json',
        );
        callback(null, {
          status_code: injected.statusCode,
          content_type: contentType,
          body_json: Buffer.from(injected.rawPayload),
        });
      } catch (error) {
        const details =
          error instanceof Error ? error.message : 'AGENT_GRPC_INTERNAL';
        callback({ code: grpc.status.INTERNAL, details });
      }
    },

    StreamRunEvents: async (
      call: grpc.ServerWritableStream<StreamRunEventsRequest, RunEventFrame>,
    ) => {
      const authErr = verifyServiceMetadata(call.metadata, expectedAuth);
      if (authErr) {
        call.destroy(
          Object.assign(new Error(authErr.details ?? 'UNAUTHORIZED'), authErr),
        );
        return;
      }
      const req = call.request;
      const runId = req.run_id?.trim() ?? '';
      if (!runId) {
        call.destroy(
          Object.assign(new Error('MISSING_RUN_ID'), {
            code: grpc.status.INVALID_ARGUMENT,
          }),
        );
        return;
      }
      const identity = parseStreamIdentity(req.query);
      if (!identity) {
        call.destroy(
          Object.assign(new Error('MISSING_IDENTITY'), {
            code: grpc.status.INVALID_ARGUMENT,
          }),
        );
        return;
      }

      const abort = new AbortController();
      const onCancel = () => abort.abort();
      call.once('cancelled', onCancel);
      call.once('close', onCancel);

      let cursor: string | undefined =
        req.last_event_id?.trim() || req.query?.after?.trim() || undefined;
      try {
        // Validate run scope & cursor before streaming frames.
        let batch = await service.events(runId, identity, cursor);
        while (!abort.signal.aborted && !call.cancelled) {
          for (const event of batch) {
            if (abort.signal.aborted || call.cancelled) break;
            cursor = event.cursor;
            call.write({
              id: event.cursor,
              event: event.type,
              data_json: Buffer.from(JSON.stringify(event.payload), 'utf8'),
            });
          }
          if (abort.signal.aborted || call.cancelled) break;
          if (service.waitForEvents) {
            await service.waitForEvents(
              runId,
              streamPollFallbackMs,
              abort.signal,
            );
          } else {
            await delay(streamPollFallbackMs, undefined, {
              signal: abort.signal,
            });
          }
          if (abort.signal.aborted || call.cancelled) break;
          batch = await service.events(runId, identity, cursor);
        }
        if (!call.destroyed) call.end();
      } catch (error) {
        if (abort.signal.aborted || call.cancelled) return;
        if (error instanceof AgentError) {
          call.destroy(
            Object.assign(new Error(error.code), {
              code: grpcCodeForAgentError(error),
            }),
          );
          return;
        }
        const message =
          error instanceof Error ? error.message : 'STREAM_FAILED';
        call.destroy(
          Object.assign(new Error(message), { code: grpc.status.INTERNAL }),
        );
      } finally {
        call.off('cancelled', onCancel);
        call.off('close', onCancel);
      }
    },
  });

  const boundPort = await new Promise<number>((resolve, reject) => {
    server.bindAsync(
      address,
      grpc.ServerCredentials.createInsecure(),
      (err, port) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(port);
      },
    );
  });

  const host = address.includes(':')
    ? address.slice(0, address.lastIndexOf(':'))
    : '127.0.0.1';
  return {
    address: `${host}:${boundPort}`,
    close: () =>
      new Promise<void>((resolve) => {
        server.tryShutdown(() => resolve());
      }),
  };
}
