import "reflect-metadata";
import { timingSafeEqual } from "node:crypto";
import { createServer, type IncomingMessage } from "node:http";
import { pathToFileURL } from "node:url";
import { Body, Controller, Get, Module, Post, Req, Res } from "@nestjs/common";
import { NestFactory } from "@nestjs/core";
import {
  FastifyAdapter,
  type NestFastifyApplication,
} from "@nestjs/platform-fastify";
import type { FastifyReply, FastifyRequest } from "fastify";
import { computeCandidate } from "./service-runtime.js";

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
class RuntimeController {
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
      const failure = error && typeof error === "object" ? error : {};
      const definitive = "definitive" in failure && failure.definitive === true;
      return send(reply, definitive ? 422 : 503, {
        code:
          "code" in failure && typeof failure.code === "string"
            ? failure.code
            : "RUNTIME_UNAVAILABLE",
        definitive,
      });
    }
  }
}

@Module({ controllers: [RuntimeController] })
class RuntimeModule {}

export async function createRuntime({
  token,
  maxConcurrent = 4,
}: {
  token: string | undefined;
  maxConcurrent?: number;
}): Promise<NestFastifyApplication> {
  if (typeof token !== "string" || Buffer.byteLength(token) < 32)
    throw new Error("RUNTIME_TOKEN_REQUIRED");

  const expected = Buffer.from(`Bearer ${token}`);
  let active = 0;
  const adapter = new FastifyAdapter({
    requestTimeout: 15000,
    exposeHeadRoutes: false,
    serverFactory: (handler) =>
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
    const actual = Buffer.from(request.headers.authorization ?? "");
    if (
      actual.length !== expected.length ||
      !timingSafeEqual(actual, expected)
    ) {
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
    if (active >= maxConcurrent) {
      request.raw.resume();
      return send(reply, 503, { code: "BUSY", definitive: false });
    }
    active++;
    reply.raw.once("close", () => active--);
  });

  const app = await NestFactory.create<NestFastifyApplication>(
    RuntimeModule,
    adapter,
    { bodyParser: false, logger: false, abortOnError: false },
  );
  await app.init();
  return app;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const app = await createRuntime({
    token: process.env.GLOWLOOM_COLLAB_RUNTIME_TOKEN,
  });
  app.enableShutdownHooks(["SIGTERM", "SIGINT"]);
  await app.listen(
    Number(process.env.PORT ?? 4400),
    process.env.HOST ?? "127.0.0.1",
  );
}
