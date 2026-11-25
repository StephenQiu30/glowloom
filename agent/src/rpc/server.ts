import { timingSafeEqual } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import type { NestFastifyApplication } from '@nestjs/platform-fastify';

const maxMessageBytes = 8 * 1024 * 1024;

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
  const rawAuth = typeof authValues[0] === 'string' ? authValues[0] : '';
  const actualAuth = Buffer.from(rawAuth);
  if (
    actualAuth.length !== expectedAuth.length ||
    !timingSafeEqual(actualAuth, expectedAuth)
  ) {
    return { code: grpc.status.UNAUTHENTICATED, details: 'UNAUTHORIZED' };
  }
  const audValues = metadata.get('x-agent-audience');
  const aud = typeof audValues[0] === 'string' ? audValues[0] : '';
  if (aud !== 'agent') {
    return {
      code: grpc.status.PERMISSION_DENIED,
      details: 'INVALID_AUDIENCE',
    };
  }
  return null;
}

export async function startAgentGrpcServer({
  app,
  address,
  serviceToken,
}: {
  app: NestFastifyApplication;
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
      const runId = req.run_id ?? '';
      if (!runId) {
        call.destroy(
          Object.assign(new Error('MISSING_RUN_ID'), {
            code: grpc.status.INVALID_ARGUMENT,
          }),
        );
        return;
      }
      try {
        const qs = new URLSearchParams(req.query ?? {}).toString();
        const url = `/internal/agent/runs/${encodeURIComponent(runId)}/events${qs ? `?${qs}` : ''}`;
        const headers: Record<string, string> = {
          authorization: `Bearer ${serviceToken}`,
          'x-agent-audience': 'agent',
        };
        if (req.last_event_id) {
          headers['last-event-id'] = req.last_event_id;
        }
        const injected = await fastify.inject({
          method: 'GET',
          url,
          headers,
        });
        if (injected.statusCode !== 200) {
          call.destroy(
            Object.assign(new Error(`HTTP_${injected.statusCode}`), {
              code: grpc.status.FAILED_PRECONDITION,
            }),
          );
          return;
        }
        const blocks = injected.body.split('\n\n');
        for (const block of blocks) {
          if (call.cancelled) break;
          const trimmed = block.trim();
          if (!trimmed || trimmed.startsWith(':')) continue;
          let id = '';
          let event = 'message';
          const dataLines: string[] = [];
          for (const line of trimmed.split('\n')) {
            if (line.startsWith('id:')) id = line.slice(3).trim();
            else if (line.startsWith('event:')) event = line.slice(6).trim();
            else if (line.startsWith('data:'))
              dataLines.push(line.slice(5).trim());
          }
          if (dataLines.length > 0) {
            call.write({
              id,
              event,
              data_json: Buffer.from(dataLines.join('\n'), 'utf8'),
            });
          }
        }
        call.end();
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'STREAM_FAILED';
        call.destroy(
          Object.assign(new Error(message), { code: grpc.status.INTERNAL }),
        );
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
