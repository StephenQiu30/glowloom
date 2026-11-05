import { Value } from "typebox/value";
import type { TSchema, Static } from "typebox";
import { timingSafeEqual } from "node:crypto";
import { Readable } from "node:stream";
import { setTimeout as delay } from "node:timers/promises";
import type { Logger } from "pino";
import Fastify from "fastify";
import swagger from "@fastify/swagger";
import type { TypeBoxTypeProvider } from "@fastify/type-provider-typebox";
import {
  CreateSessionSchema,
  CreateRunSchema,
  ControlSchema,
  SessionViewSchema,
  RunSnapshotSchema,
  AcceptedRunSchema,
  ControlReceiptSchema,
  SessionListSchema,
  IdentityQuery,
  HistoryQuery,
  MessageQuery,
  SubmissionParams,
  MessageParams,
  RunPageSchema,
  MessagePageSchema,
  PublicMessageSchema,
  CurrentSubmissionSchema,
  SessionParams,
  RunParams,
  operationIds,
  type CreateSessionInput,
  type CreateRunInput,
  type ControlInput,
  type Identity,
  type HistoryInput,
  type MessageInput,
} from "../../contracts/agent.js";

export interface RuntimeEvent {
  cursor: string;
  type: string;
  payload: unknown;
}
export interface AgentApplication {
  ready(): Promise<boolean>;
  createSession(input: CreateSessionInput): Promise<unknown>;
  listSessions(identity: HistoryInput): Promise<unknown>;
  readSession(id: string, identity: Identity): Promise<unknown>;
  createRun(sessionId: string, input: CreateRunInput): Promise<unknown>;
  readRun(id: string, identity: Identity): Promise<unknown>;
  command(id: string, input: ControlInput): Promise<unknown>;
  events(
    id: string,
    identity: Identity,
    after: string | undefined,
  ): Promise<RuntimeEvent[]>;
  close(): Promise<void>;
  listRuns(sessionId: string, query: HistoryInput): Promise<unknown>;
  readSubmission(
    sessionId: string,
    requestId: string,
    identity: Identity,
  ): Promise<unknown>;
  currentSubmission(sessionId: string, identity: Identity): Promise<unknown>;
  listMessages(runId: string, query: MessageInput): Promise<unknown>;
  readPublicMessage(
    runId: string,
    messageId: string,
    identity: Identity,
  ): Promise<unknown>;
}
export class HttpProblem extends Error {
  constructor(
    readonly statusCode: number,
    readonly code: string,
  ) {
    super(code);
  }
}
function checked<S extends TSchema>(schema: S, value: unknown): Static<S> {
  const json: unknown = JSON.parse(JSON.stringify(value));
  if (!Value.Check(schema, json))
    throw new HttpProblem(500, "invalid_service_response");
  return json;
}
export async function buildApp(options: {
  service: AgentApplication;
  serviceToken: string;
  logger?: Logger;
}) {
  const app = Fastify({
    bodyLimit: 8 * 1024 * 1024,
    requestTimeout: 30000,
    ...(options.logger
      ? { loggerInstance: options.logger }
      : { logger: false }),
    ajv: { customOptions: { removeAdditional: false } },
  }).withTypeProvider<TypeBoxTypeProvider>();
  await app.register(swagger, {
    openapi: {
      openapi: "3.0.3",
      info: { title: "Glowloom agent internal API", version: "1" },
      components: {
        securitySchemes: {
          serviceIdentity: { type: "http", scheme: "bearer" },
        },
      },
    },
  });
  app.addHook("onRequest", async (req, reply) => {
    if (!req.url.startsWith("/internal/")) return;
    const supplied = Buffer.from(
      req.headers.authorization?.replace(/^Bearer /, "") ?? "",
    );
    const expected = Buffer.from(options.serviceToken);
    if (
      req.headers["x-agent-audience"] !== "agent" ||
      supplied.length !== expected.length ||
      !timingSafeEqual(supplied, expected)
    ) {
      return reply.code(401).send({ code: "service_identity_required" });
    }
  });
  app.setErrorHandler((error, req, reply) => {
    const e = error as {
      validation?: unknown;
      statusCode?: number;
      code?: string;
    };
    const status = e.validation
      ? 400
      : e.statusCode && e.statusCode >= 400 && e.statusCode < 600
        ? e.statusCode
        : 500;
    return reply.code(status).send({
      code: e.validation
        ? "invalid_request"
        : error instanceof HttpProblem
          ? error.code
          : status === 500
            ? "internal_error"
            : "request_failed",
      request_id: req.id,
    });
  });
  const service = options.service;
  const secure = { security: [{ serviceIdentity: [] }] };
  app.get("/healthz", async () => ({ status: "ok", service: "agent" }));
  app.get("/readyz", async (_req, reply) => {
    const ready = await service.ready();
    return reply.code(ready ? 200 : 503).send({ ready });
  });
  app.post(
    "/internal/agent/sessions",
    {
      schema: {
        ...secure,
        operationId: operationIds.createSession,
        body: CreateSessionSchema,
        response: { 201: SessionViewSchema },
      },
    },
    async (req, reply) =>
      reply
        .code(201)
        .send(
          checked(SessionViewSchema, await service.createSession(req.body)),
        ),
  );
  app.get(
    "/internal/agent/sessions",
    {
      schema: {
        ...secure,
        operationId: operationIds.listSessions,
        querystring: HistoryQuery,
        response: { 200: SessionListSchema },
      },
    },
    async (req) =>
      checked(SessionListSchema, await service.listSessions(req.query)),
  );
  app.get(
    "/internal/agent/sessions/:sid",
    {
      schema: {
        ...secure,
        operationId: operationIds.readSession,
        params: SessionParams,
        querystring: IdentityQuery,
        response: { 200: SessionViewSchema },
      },
    },
    async (req) => {
      const result = await service.readSession(req.params.sid, req.query);
      if (result === undefined) throw new HttpProblem(404, "session_not_found");
      return checked(SessionViewSchema, result);
    },
  );
  app.get(
    "/internal/agent/sessions/:sid/runs",
    {
      schema: {
        ...secure,
        operationId: operationIds.listRuns,
        params: SessionParams,
        querystring: HistoryQuery,
        response: { 200: RunPageSchema },
      },
    },
    async (req) =>
      checked(RunPageSchema, await service.listRuns(req.params.sid, req.query)),
  );
  app.get(
    "/internal/agent/sessions/:sid/submissions/current",
    {
      schema: {
        ...secure,
        operationId: operationIds.currentSubmission,
        params: SessionParams,
        querystring: IdentityQuery,
        response: { 200: CurrentSubmissionSchema },
      },
    },
    async (req) =>
      checked(
        CurrentSubmissionSchema,
        await service.currentSubmission(req.params.sid, req.query),
      ),
  );
  app.get(
    "/internal/agent/sessions/:sid/submissions/:requestId",
    {
      schema: {
        ...secure,
        operationId: operationIds.readSubmission,
        params: SubmissionParams,
        querystring: IdentityQuery,
        response: { 200: AcceptedRunSchema },
      },
    },
    async (req) => {
      const value = await service.readSubmission(
        req.params.sid,
        req.params.requestId,
        req.query,
      );
      if (!value) throw new HttpProblem(404, "submission_not_found");
      return checked(AcceptedRunSchema, value);
    },
  );
  app.get(
    "/internal/agent/runs/:rid/messages",
    {
      schema: {
        ...secure,
        operationId: operationIds.listMessages,
        params: RunParams,
        querystring: MessageQuery,
        response: { 200: MessagePageSchema },
      },
    },
    async (req) =>
      checked(
        MessagePageSchema,
        await service.listMessages(req.params.rid, req.query),
      ),
  );
  app.get(
    "/internal/agent/runs/:rid/messages/:messageId",
    {
      schema: {
        ...secure,
        operationId: operationIds.readMessage,
        params: MessageParams,
        querystring: IdentityQuery,
        response: { 200: PublicMessageSchema },
      },
    },
    async (req) => {
      const value = await service.readPublicMessage(
        req.params.rid,
        req.params.messageId,
        req.query,
      );
      if (!value) throw new HttpProblem(404, "message_not_found");
      return checked(PublicMessageSchema, value);
    },
  );
  app.post(
    "/internal/agent/sessions/:sid/runs",
    {
      schema: {
        ...secure,
        operationId: operationIds.createRun,
        params: SessionParams,
        body: CreateRunSchema,
        response: { 202: AcceptedRunSchema },
      },
    },
    async (req, reply) =>
      reply
        .code(202)
        .send(
          checked(
            AcceptedRunSchema,
            await service.createRun(req.params.sid, req.body),
          ),
        ),
  );
  app.get(
    "/internal/agent/runs/:rid",
    {
      schema: {
        ...secure,
        operationId: operationIds.readRun,
        params: RunParams,
        querystring: IdentityQuery,
        response: { 200: RunSnapshotSchema },
      },
    },
    async (req) => {
      const result = await service.readRun(req.params.rid, req.query);
      if (result === undefined) throw new HttpProblem(404, "run_not_found");
      return checked(RunSnapshotSchema, result);
    },
  );
  app.post(
    "/internal/agent/runs/:rid/commands",
    {
      schema: {
        ...secure,
        operationId: operationIds.command,
        params: RunParams,
        body: ControlSchema,
        response: { 200: ControlReceiptSchema },
      },
    },
    async (req) =>
      checked(
        ControlReceiptSchema,
        await service.command(req.params.rid, req.body),
      ),
  );
  app.get(
    "/internal/agent/runs/:rid/events",
    {
      schema: {
        ...secure,
        operationId: operationIds.events,
        params: RunParams,
        querystring: IdentityQuery,
      },
    },
    async (req, reply) => {
      const initial = req.headers["last-event-id"];
      let cursor = Array.isArray(initial) ? initial[0] : initial;
      // Resolve authentication/scope and cursor errors before streaming headers.
      const first = await service.events(req.params.rid, req.query, cursor);
      const controller = new AbortController();
      reply.raw.once("close", () => controller.abort());
      async function* stream() {
        let batch = first;
        while (!controller.signal.aborted) {
          for (const event of batch) {
            cursor = event.cursor;
            yield `id: ${event.cursor}\nevent: ${event.type}\ndata: ${JSON.stringify(event.payload)}\n\n`;
          }
          if (controller.signal.aborted) break;
          try {
            await delay(1000, undefined, { signal: controller.signal });
          } catch {
            break;
          }
          if (controller.signal.aborted) break;
          batch = await service.events(req.params.rid, req.query, cursor);
          if (batch.length === 0) yield ": keepalive\n\n";
        }
      }
      return reply
        .header("content-type", "text/event-stream")
        .header("cache-control", "no-cache, no-transform")
        .header("x-accel-buffering", "no")
        .send(Readable.from(stream()));
    },
  );
  app.addHook("onClose", () => service.close());
  await app.ready();
  return app;
}
