import { verificationCommand } from "./verification";
import { createServer } from "node:http";
import { prepareSocket } from "./socket";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { readFile, mkdir, chmod, realpath, rm } from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { createHash, timingSafeEqual } from "node:crypto";
import { CodexHost, type TaskResult } from "../executors/codex";
import { atomicWrite, readJson, claimJson } from "./files";
import { readDocument, applyProposal } from "./proposals";
import { liveContext, currentContext } from "./context";
import { digest } from "../../content/parse";
import { parseWorkflow, type TaskContext } from "../../workflows/definition";
import type { Evidence } from "../../workflows/run";
const execute = promisify(execFile);
type Journal = {
  requestDigest: string;
  status: string;
  threadId?: string;
  turnId?: string;
  result?: TaskResult;
};
export async function startBridge(options: {
  repo: string;
  storage: string;
  socket: string;
  token: string;
  codexBinary: string;
}) {
  if (options.token.length < 32)
    throw new Error("Bridge token must be configured");
  const repo = await realpath(options.repo);
  const storage = resolve(options.storage);
  await mkdir(storage, { recursive: true, mode: 0o700 });
  await mkdir(dirname(options.socket), { recursive: true, mode: 0o700 });
  let host: CodexHost | undefined;
  const executor = () => (host ??= new CodexHost(options.codexBinary));
  const journalPath = (run: string, attempt: number) => {
    if (
      !/^[a-f0-9-]{36}$/.test(run) ||
      !Number.isSafeInteger(attempt) ||
      attempt < 1
    )
      throw new Error("invalid_run");
    return resolve(storage, "runs", `${run}-${attempt}.json`);
  };
  const query = async (run: string, attempt: number): Promise<TaskResult> => {
    const path = journalPath(run, attempt),
      journal = await readJson<Journal>(path);
    if (!journal || !journal.threadId) return { status: "unknown" };
    if (
      journal.result &&
      ["completed", "failed"].includes(journal.result.status)
    )
      return journal.result;
    const result = await executor().readTask({
      threadId: journal.threadId,
      turnId: journal.turnId ?? "",
    });
    journal.result = result;
    journal.status = result.status;
    await atomicWrite(path, JSON.stringify(journal));
    return result;
  };
  const route = async (
    path: string,
    body: Record<string, unknown>,
  ): Promise<unknown> => {
    if (path === "/health")
      return { connected: true, executor: "codex", tts: false };
    if (path === "/context") {
      return liveContext(
        repo,
        String(body.project),
        String(body.workflowId),
        Array.isArray(body.documents)
          ? body.documents.filter((d): d is string => typeof d === "string")
          : [],
      );
    }
    if (path === "/context/check")
      return currentContext(
        repo,
        body.context as TaskContext,
        String(body.workflowId),
      );
    if (path === "/document/read") {
      const d = await readDocument(repo, String(body.documentId));
      return { id: d.id, source: d.text, revision: d.revision };
    }
    if (path === "/host/skills") {
      const live = await liveContext(
        repo,
        String(body.project ?? "workspace"),
        "feature",
      );
      const skills = (await executor().skills(live.projectPath)).filter((s) =>
        s.name.startsWith("workspace-"),
      );
      return skills.map(({ name, path, enabled }) => ({
        name,
        sourcePath: path.startsWith(repo + "/")
          ? path.slice(repo.length + 1)
          : "external",
        enabled,
      }));
    }
    if (path === "/proposal/apply")
      return applyProposal(repo, storage, {
        id: String(body.id),
        documentId: String(body.documentId),
        baseRevision: String(body.baseRevision),
        text: String(body.text),
        actor: String(body.actor),
      });
    if (path === "/run/query")
      return query(String(body.runId), Number(body.attempt));
    if (path === "/run/interrupt") {
      const file = journalPath(String(body.runId), Number(body.attempt));
      await atomicWrite(
        file + ".cancel.json",
        JSON.stringify({ requestedAt: new Date().toISOString() }),
      );
      const journal = await readJson<Journal>(file);
      if (journal?.threadId && journal.turnId)
        await executor().interrupt({
          threadId: journal.threadId,
          turnId: journal.turnId,
        });
      return {
        status: "cancel_requested",
        handle:
          journal?.threadId && journal.turnId
            ? `${journal.threadId}:${journal.turnId}`
            : null,
        sideEffectsReverted: false,
      };
    }
    if (path === "/run/start") {
      const context = body.context as TaskContext;
      const workflowId = String(body.workflowId);
      const current = await currentContext(repo, context, workflowId);
      if (!current.current) throw new Error("inputs_stale");
      const path = journalPath(String(body.runId), Number(body.attempt));
      if (await readJson(path + ".cancel.json"))
        return { status: "failed", result: "Canceled before dispatch" };
      const requestDigest = digest(
        JSON.stringify([context.digest, workflowId, body.stage]),
      );
      const existing = await readJson<Journal>(path);
      if (existing) {
        if (existing.requestDigest !== requestDigest)
          throw new Error("idempotency_conflict");
        return query(String(body.runId), Number(body.attempt));
      }
      const workflow = parseWorkflow(current.live.workflowSource);
      const stage = workflow.stages[Number(body.stage)];
      if (!stage) throw new Error("unknown_stage");
      const journal: Journal = { requestDigest, status: "dispatching" };
      if (!(await claimJson(path, journal)))
        return query(String(body.runId), Number(body.attempt));
      try {
        const handle = await executor().startTask(
          current.live.projectPath,
          context,
          stage.label,
          async (id) => {
            journal.threadId = id;
            await atomicWrite(path, JSON.stringify(journal));
            if (await readJson(path + ".cancel.json"))
              throw new Error("canceled_before_turn");
          },
        );
        Object.assign(journal, handle, { status: "running" });
        await atomicWrite(path, JSON.stringify(journal));
        if (await readJson(path + ".cancel.json"))
          await executor().interrupt(handle);
        return {
          status: "running",
          handle: `${handle.threadId}:${handle.turnId}`,
        };
      } catch {
        journal.status = "unknown";
        await atomicWrite(path, JSON.stringify(journal));
        return { status: "unknown", handle: journal.threadId };
      }
    }
    if (path === "/check/query") {
      const key = digest(
        String(body.runId) + ":" + String(body.idempotencyKey),
      );
      const saved = await readJson<{ status: string; evidence?: Evidence }>(
        resolve(storage, "checks", key + ".json"),
      );
      if (saved?.status === "ready" && saved.evidence) return saved.evidence;
      throw new Error("verification_unknown");
    }
    if (path === "/check") {
      const context = body.context as TaskContext;
      const workflowId = String(body.workflowId);
      let current;
      try {
        current = await currentContext(repo, context, workflowId);
      } catch {
        return { notDispatched: true, reason: "context_unavailable" };
      }
      const commandId = String(body.commandId);
      if (!current.current || !context.allowedActions.includes("verify"))
        return { notDispatched: true, reason: "inputs_stale" };
      let command;
      try {
        command = verificationCommand(context.project, commandId);
      } catch (error) {
        return { notDispatched: true, reason: (error as Error).message };
      }
      const key = digest(
        String(body.runId) + ":" + String(body.idempotencyKey),
      );
      const path = resolve(storage, "checks", key + ".json");
      const saved = await readJson<{
        status: string;
        requestDigest: string;
        evidence?: Evidence;
      }>(path);
      const requestDigest = digest(context.digest + ":" + commandId);
      if (saved) {
        if (saved.requestDigest !== requestDigest)
          throw new Error("idempotency_conflict");
        if (saved.status === "ready") return saved.evidence;
        throw new Error("verification_unknown");
      }
      if (!(await claimJson(path, { status: "running", requestDigest })))
        throw new Error("verification_unknown");
      const report = resolve(storage, "checks", key + "-report.json");
      const args = [...command.args];
      if (commandId === "unit") args.push(`--outputFile=${report}`);
      const env: NodeJS.ProcessEnv = {
        NODE_ENV: "test",
        PATH: process.env.PATH,
        HOME: process.env.HOME,
        LANG: process.env.LANG,
      };
      if (process.env.GLOWLOOM_WORKSPACE_TEST_DB_DSN)
        env.GLOWLOOM_WORKSPACE_TEST_DB_DSN =
          process.env.GLOWLOOM_WORKSPACE_TEST_DB_DSN;
      let output = "",
        passed = false;
      try {
        const result = await execute(command.binary, args, {
          cwd: current.live.projectPath,
          env,
          timeout: 240000,
          maxBuffer: 10_000_000,
        });
        output = result.stdout + result.stderr;
        passed = true;
      } catch (error) {
        const e = error as { stdout?: string; stderr?: string };
        output = (e.stdout ?? "") + (e.stderr ?? "");
      }
      let status: Evidence["status"] = passed ? "passed" : "failed";
      if (commandId === "unit" && passed) {
        try {
          const summary = JSON.parse(await readFile(report, "utf8"));
          if (summary.numPendingTests > 0) status = "not_run";
        } catch {
          status = "not_run";
        }
      }
      const hash = digest(output);
      await atomicWrite(resolve(storage, "artifacts", hash + ".txt"), output);
      const evidence: Evidence = {
        kind: "command",
        status,
        inputDigest: context.digest,
        summary: `${commandId}: ${status}; ${output.slice(-2500)}`,
        artifactRef: "sha256:" + hash,
      };
      await atomicWrite(
        path,
        JSON.stringify({ status: "ready", requestDigest, evidence }),
      );
      return evidence;
    }
    if (path === "/artifact/read") {
      const hash = String(body.hash);
      if (!/^[a-f0-9]{64}$/.test(hash)) throw new Error("invalid_artifact");
      const text = await readFile(
        resolve(storage, "artifacts", hash + ".txt"),
        "utf8",
      );
      if (digest(text) !== hash) throw new Error("artifact_mismatch");
      return { text, checksum: hash };
    }
    throw new Error("not_found");
  };
  const server = createServer(async (req, res) => {
    res.setHeader("Content-Type", "application/json");
    const given = req.headers.authorization ?? "";
    const hash = (s: string) => createHash("sha256").update(s).digest();
    if (!timingSafeEqual(hash(given), hash("Bearer " + options.token))) {
      res.writeHead(401).end(JSON.stringify({ error: "unauthorized" }));
      return;
    }
    if (req.method !== "POST") {
      res.writeHead(405).end(JSON.stringify({ error: "method_not_allowed" }));
      return;
    }
    try {
      const chunks: Buffer[] = [];
      let size = 0;
      for await (const chunk of req) {
        const bytes = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
        size += bytes.length;
        if (size > 2_500_000) throw new Error("body_too_large");
        chunks.push(bytes);
      }
      const parsed: unknown = JSON.parse(
        new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks)),
      );
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
        throw new Error("invalid_body");
      const result = await route(
        req.url ?? "",
        parsed as Record<string, unknown>,
      );
      res.end(JSON.stringify(result));
    } catch (error) {
      const code = error instanceof Error ? error.message : "bridge_error";
      const known =
        /^(?:revision_conflict|idempotency_conflict|application_unknown|document_busy|inputs_stale|proposal_links_invalid|verification_unknown|verification_command_not_configured|unknown_[\w]+|document_[\w]+|invalid_[\w]+|artifact_mismatch|body_too_large|not_found)$/;
      res.writeHead(known.test(code) ? 409 : 503).end(
        JSON.stringify({
          error: known.test(code) ? code : "executor_unavailable",
        }),
      );
    }
  });
  await prepareSocket(options.socket);
  await new Promise<void>((yes, no) => {
    server.once("error", no);
    server.listen(options.socket, yes);
  });
  await chmod(options.socket, 0o600);
  return {
    server,
    close: async () => {
      host?.close();
      await new Promise<void>((yes) => server.close(() => yes()));
      await rm(options.socket, { force: true });
    },
  };
}
