import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { InMemoryEventBus } from '../src/sessions/event-bus.js';
import type {
  AgentApplication,
  RunEvent,
  RunView,
  SessionView,
} from '../src/sessions/types.js';
import {
  startAgentGrpcServer,
  type AgentGrpcServer,
} from '../src/transport/grpc-server.js';
import {
  startHealthServer,
  type HealthServer,
} from '../src/transport/health-server.js';

const serviceToken = 't'.repeat(32);

function loadRuntimeClient(address: string) {
  const dir = path.dirname(fileURLToPath(import.meta.url));
  const protoPath = path.resolve(
    dir,
    '../../backend/api/proto/glowloom/agent/runtime.proto',
  );
  expect(existsSync(protoPath)).toBe(true);
  const def = protoLoader.loadSync(protoPath, {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: false,
    oneofs: true,
  });
  const pkg = grpc.loadPackageDefinition(def) as {
    glowloom: { agent: { AgentRuntime: grpc.ServiceClientConstructor } };
  };
  return new pkg.glowloom.agent.AgentRuntime(
    address,
    grpc.credentials.createInsecure(),
  );
}

describe('AgentRuntime strongly-typed gRPC and minimal HTTP health servers', () => {
  const eventBus = new InMemoryEventBus();
  let healthServer: HealthServer;
  let grpcServer: AgentGrpcServer;

  const stubService: AgentApplication = {
    async ready() {
      return true;
    },
    async createSession(input): Promise<SessionView> {
      return {
        id: 'sess-1',
        actor_id: input.identity.actorId,
        org_id: input.identity.orgId ?? input.identity.actorId,
        scope_id: input.identity.scopeId ?? input.identity.projectId,
        project_id: input.identity.projectId,
        title: input.title,
        execution_revision: 0,
        pinned: false,
        created_at: new Date().toISOString(),
      };
    },
    async listSessions() {
      return { items: [] };
    },
    async getSession(_id, sessionId): Promise<SessionView> {
      return {
        id: sessionId,
        actor_id: 'actor-1',
        org_id: 'org-1',
        scope_id: 'scope-1',
        project_id: 'proj-1',
        title: 'Demo',
        execution_revision: 1,
        pinned: false,
        created_at: new Date().toISOString(),
      };
    },
    async updateSession(input): Promise<SessionView> {
      return {
        id: input.sessionId,
        actor_id: input.identity.actorId,
        org_id: 'org-1',
        scope_id: 'scope-1',
        project_id: input.identity.projectId,
        title: input.title ?? 'Demo',
        execution_revision: 1,
        pinned: Boolean(input.pinned),
        created_at: new Date().toISOString(),
      };
    },
    async startRun(input) {
      return {
        root_run_id: 'run-1',
        session_id: input.sessionId,
        submission_id: 'sub-1',
        accepted_revision: 1,
        status: 'running',
      };
    },
    async listRuns() {
      return { items: [] };
    },
    async getRun(_id, runId): Promise<RunView> {
      return {
        root_run_id: runId,
        session_id: 'sess-1',
        submission_id: 'sub-1',
        goal: 'plan',
        manifest: {},
        execution_available: true,
        state: { status: 'completed', execution_revision: 1 },
        snapshot_cursor: '1:0',
        messages_page: { items: [], through: '1' },
        events: [],
      };
    },
    async getCurrentSubmission() {
      return {
        active_run: null,
        latest_accepted: null,
        unresolved_run_ids: [],
      };
    },
    async getSubmission() {
      return { root_run_id: 'run-1' };
    },
    async listMessages() {
      return { items: [], through: '0' };
    },
    async getMessage(_id, runId, messageId) {
      return {
        message_id: messageId,
        run_id: runId,
        conversation_id: 1,
        sequence: '1',
        part_index: 0,
        kind: 'assistant',
        content: 'ok',
      };
    },
    async applyDecision(input) {
      return { decision_id: input.decisionId, applied: true };
    },
    async listRunEventsAfter(
      _id,
      runId,
    ): Promise<{
      events: RunEvent[];
      terminal: boolean;
    }> {
      return {
        events: [
          {
            id: '1',
            seq: 1,
            runId,
            event: 'run.accepted',
            data: { root_run_id: runId },
          },
          {
            id: '2',
            seq: 2,
            runId,
            event: 'run.completed',
            data: { root_run_id: runId },
          },
        ],
        terminal: true,
      };
    },
    subscribeRunEvents(runId, handler) {
      return eventBus.subscribe(runId, handler);
    },
    startRecovery() {},
    async stopRecovery() {},
  };

  beforeAll(async () => {
    healthServer = await startHealthServer({
      host: '127.0.0.1',
      port: 0,
      service: stubService,
    });
    grpcServer = await startAgentGrpcServer({
      service: stubService,
      address: '127.0.0.1:0',
      serviceToken,
    });
  });

  afterAll(async () => {
    await grpcServer.close();
    await healthServer.close();
    await eventBus.close();
  });

  it('serves /healthz and /readyz on minimal HTTP health server', async () => {
    const res = await fetch(`http://${healthServer.address}/healthz`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { status: string; service: string };
    expect(body).toEqual({ status: 'ok', service: 'agent' });
  });

  it('rejects unauthenticated gRPC calls and handles CreateSession + StreamRunEvents', async () => {
    const client = loadRuntimeClient(grpcServer.address) as unknown as {
      CreateSession(
        req: Record<string, unknown>,
        md: grpc.Metadata,
        cb: (
          err: grpc.ServiceError | null,
          res?: { status_code: number; payload_json: Buffer },
        ) => void,
      ): void;
      StreamRunEvents(
        req: Record<string, unknown>,
        md: grpc.Metadata,
      ): grpc.ClientReadableStream<{
        id: string;
        event: string;
        data_json: Buffer;
      }>;
      close(): void;
    };

    try {
      const unauthErr = await new Promise<grpc.ServiceError | null>(
        (resolve) => {
          client.CreateSession(
            {
              identity: { actor_id: 'a', project_id: 'p' },
              request_id: 'r1',
              title: 'T',
            },
            new grpc.Metadata(),
            (err) => resolve(err),
          );
        },
      );
      expect(unauthErr?.code).toBe(grpc.status.UNAUTHENTICATED);

      const md = new grpc.Metadata();
      md.set('authorization', `Bearer ${serviceToken}`);
      md.set('x-agent-audience', 'agent');

      const created = await new Promise<{
        status_code: number;
        payload_json: Buffer;
      }>((resolve, reject) => {
        client.CreateSession(
          {
            identity: { actor_id: 'actor-1', project_id: 'proj-1' },
            request_id: 'req-1',
            title: 'Episode 1',
          },
          md,
          (err, res) => (err || !res ? reject(err) : resolve(res)),
        );
      });
      expect(created.status_code).toBe(200);
      expect(JSON.parse(created.payload_json.toString('utf8')).title).toBe(
        'Episode 1',
      );

      const frames: Array<{ id: string; event: string }> = [];
      await new Promise<void>((resolve, reject) => {
        const stream = client.StreamRunEvents(
          {
            identity: { actor_id: 'actor-1', project_id: 'proj-1' },
            run_id: 'run-1',
          },
          md,
        );
        stream.on('data', (frame) =>
          frames.push({ id: frame.id, event: frame.event }),
        );
        stream.on('error', reject);
        stream.on('end', () => resolve());
      });

      expect(frames).toEqual([
        { id: '1', event: 'run.accepted' },
        { id: '2', event: 'run.completed' },
      ]);
    } finally {
      client.close();
    }
  });
});
