import "reflect-metadata";
import {
  createServer,
  type IncomingMessage,
  type RequestListener,
} from "node:http";
import { Body, Controller, Get, Module, Post, Req, Res } from "@nestjs/common";
import { NestFactory } from "@nestjs/core";
import {
  FastifyAdapter,
  type NestFastifyApplication,
} from "@nestjs/platform-fastify";
import type { FastifyReply, FastifyRequest } from "fastify";
import { computeCandidate } from "../runtime/candidate.js";
import { buildExpectedBearer, verifyBearerToken } from "./auth.js";
import { classifyRuntimeError, ConcurrencyGate } from "./concurrency.js";

const actions = ["initialize", "prepare", "rename", "commands"] as const;
const maxBodyBytes = 16 * 1024 * 1024;

function send(reply: FastifyReply, status: number, body: unknown) {
  return reply
    .header("Cache-Control", "no-store")
    .type("application/json")
    .code(status)
    .send(body);
}

@Controller()
class CanvasController {
  @Get("healthz")
  health(@Res() reply: FastifyReply) {
    return send(reply, 200, { status: "ok" });
  }

  @Post(actions.map((action) => `v1/${action}`))
  async compute(
    @Req() request: FastifyRequest,
    @Res() reply: FastifyReply,
    @Body() payload: IncomingMessage | undefined,
  ) {
    try {
      const chunks: Buffer[] = [];
      let size = 0;
      for await (const chunk of payload ?? request.raw) {
        const bytes: Buffer = chunk;
        size += bytes.length;
        if (size > maxBodyBytes)
          return send(reply, 413, {
            code: "REQUEST_TOO_LARGE",
            definitive: false,
          });
        chunks.push(bytes);
      }
      let input: unknown;
      try {
        input = JSON.parse(Buffer.concat(chunks).toString("utf8"));
      } catch {
        return send(reply, 400, { code: "INVALID_JSON", definitive: false });
      }
      return send(
        reply,
        200,
        await computeCandidate(request.url.slice(4), input),
      );
    } catch (error) {
      const { code, definitive } = classifyRuntimeError(error);
      return send(reply, definitive ? 422 : 503, { code, definitive });
    }
  }
}

@Module({ controllers: [CanvasController] })
class CanvasHttpModule {}

export async function buildApp({
  token,
  maxConcurrent = 4,
  gate,
}: {
  token: string | undefined;
  maxConcurrent?: number;
  gate?: ConcurrencyGate;
}): Promise<NestFastifyApplication> {
  const expected = buildExpectedBearer(token);
  const limiter = gate ?? new ConcurrencyGate(maxConcurrent);
  const adapter = new FastifyAdapter({
    requestTimeout: 15000,
    exposeHeadRoutes: false,
    serverFactory: (handler: RequestListener) =>
      createServer(
        { requestTimeout: 15000, headersTimeout: 5000, maxHeaderSize: 8192 },
        handler,
      ),
  });
  const server = adapter.getInstance();
  server.removeAllContentTypeParsers();
  server.addContentTypeParser("*", (_request, payload, done) =>
    done(null, payload),
  );
  server.addHook("onRequest", async (request, reply) => {
    if (request.method === "GET" && request.url === "/healthz") return;
    if (!verifyBearerToken(request.headers.authorization, expected)) {
      request.raw.resume();
      return send(reply, 401, { code: "UNAUTHORIZED" });
    }
    if (
      request.method !== "POST" ||
      !actions.some((action) => request.url === `/v1/${action}`)
    ) {
      request.raw.resume();
      return send(reply, 404, { code: "NOT_FOUND" });
    }
    const release = limiter.tryAcquire();
    if (!release) {
      request.raw.resume();
      return send(reply, 503, { code: "BUSY", definitive: false });
    }
    reply.raw.once("close", release);
  });

  const app = await NestFactory.create<NestFastifyApplication>(
    CanvasHttpModule,
    adapter,
    { bodyParser: false, logger: false, abortOnError: false },
  );
  await app.init();
  return app;
}
