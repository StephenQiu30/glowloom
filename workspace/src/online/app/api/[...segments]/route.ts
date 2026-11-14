import { verificationCommand } from "@/server/bridge/verification";
import { createHash, timingSafeEqual } from "node:crypto";
import { getSnapshot } from "@/content/runtime";
import { parseWorkflow, buildContext } from "@/workflows/definition";
import type { Action, Run, Evidence } from "@/workflows/run";
import { getConfig, getStore } from "@/server/config";
import { callBridge } from "@/server/bridge-client";
import {
  readBody,
  requireAccess,
  stringField,
  errorResponse,
  tokenFrom,
  cookieName,
} from "@/server/http";
import { issueSession, csrfFor, AccessError } from "@/server/auth/session";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const loginAttempts = { count: 0, until: 0 };
type LiveState = {
  codeDigest: string;
  repoCommit: string;
  revisions: Record<string, string>;
  skillRevisions: Record<string, string>;
  workflowRevision: string;
  workflowSource: string;
  documents: { id: string; revision: string; source: string }[];
};
type BridgeTask = {
  status: "running" | "completed" | "failed" | "unknown";
  handle?: string;
  result?: string;
};
const json = (body: unknown, status = 200) =>
  Response.json(body, { status, headers: { "Cache-Control": "no-store" } });
async function handle(request: Request) {
  const url = new URL(request.url);
  const base = process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? "";
  const path = url.pathname.slice(base.length).replace(/\/$/, "");
  const config = getConfig();
  if (path === "/api/session" && request.method === "POST") {
    if (!config.origin || request.headers.get("origin") !== config.origin)
      throw new AccessError("origin_mismatch", 403);
    if (config.secret.length < 32)
      throw new AccessError("session_unconfigured", 503);
    if (Date.now() > loginAttempts.until) {
      loginAttempts.count = 0;
      loginAttempts.until = Date.now() + 60000;
    }
    if (++loginAttempts.count > 10)
      throw new AccessError("too_many_attempts", 429);
    const body = await readBody(request);
    const token = stringField(body, "token", 1024);
    const hash = (s: string) => createHash("sha256").update(s).digest();
    if (!timingSafeEqual(hash(token), hash(config.secret)))
      throw new AccessError("invalid_token", 401);
    const session = issueSession(config.secret, [
      "read",
      "edit",
      "run",
      "accept",
    ]);
    const response = json({ csrf: csrfFor(session, config.secret) });
    response.headers.set(
      "Set-Cookie",
      `${cookieName}=${session}; Path=${base || "/"}; HttpOnly; SameSite=Strict; Max-Age=1800${config.origin.startsWith("https:") ? "; Secure" : ""}`,
    );
    return response;
  }
  const principal = requireAccess(
    request,
    path.includes("proposals")
      ? "edit"
      : path.includes("runs")
        ? "run"
        : "read",
    request.method !== "GET",
  );
  if (path === "/api/session")
    return json({
      actor: principal.actor,
      expiresAt: principal.expiresAt,
      capabilities: principal.capabilities,
      csrf: csrfFor(tokenFrom(request), config.secret),
    });
  if (path === "/api/logout" && request.method === "POST") {
    const response = json({ ok: true });
    response.headers.set(
      "Set-Cookie",
      `${cookieName}=; Path=${base || "/"}; HttpOnly; SameSite=Strict; Max-Age=0`,
    );
    return response;
  }
  if (path === "/api/status") {
    let database = false;
    try {
      if (config.database) database = await getStore().health();
    } catch {
      /* Report unavailable, never synthesize success. */
    }
    return json({
      database,
      bridge: config.socket
        ? await callBridge<{ connected: boolean }>("/health", {})
            .then((r) => r.connected)
            .catch(() => false)
        : false,
      tts: false,
      actor: principal.actor,
      mode: "local",
      message: database ? null : "在线运行存储未配置或暂不可用",
    });
  }
  if (
    path.startsWith("/api/proposals") ||
    path.startsWith("/api/runs") ||
    path.startsWith("/api/artifacts")
  )
    await getStore().health();
  if (path === "/api/host/skills" && request.method === "POST") {
    requireAccess(request, "run", true);
    const body = await readBody(request);
    return json(
      await callBridge("/host/skills", {
        project: stringField(body, "project"),
      }),
    );
  }
  if (path === "/api/proposals" && request.method === "GET")
    return json(await getStore().listProposals(principal.actor));
  if (path === "/api/proposals" && request.method === "POST") {
    const body = await readBody(request);
    const documentId = stringField(body, "documentId");
    const snapshot = await getSnapshot();
    const doc = snapshot.documents.find((d) => d.id === documentId);
    if (!doc || !doc.path.startsWith("content/") || !doc.path.endsWith(".md"))
      throw new AccessError("document_readonly", 403);
    const proposal = await getStore().createProposal({
      actor: principal.actor,
      documentId,
      baseRevision: stringField(body, "baseRevision", 64),
      text: stringField(body, "text", 2_000_000),
      baseText: typeof body.baseText === "string" ? body.baseText : undefined,
      idempotencyKey: stringField(body, "idempotencyKey"),
    });
    return json(proposal, 201);
  }
  const proposalId = path.match(
    /^\/api\/proposals\/([a-f0-9-]{36})(?:\/(apply|receipt))?$/,
  );
  if (proposalId) {
    const proposal = await getStore().getProposal(
      proposalId[1],
      principal.actor,
    );
    if (!proposal) throw new AccessError("proposal_not_found", 404);
    if (request.method === "GET") return json(proposal);
    if (request.method === "POST" && proposalId[2] === "apply") {
      if (proposal.receipt) return json(proposal);
      const receipt = await callBridge<Record<string, unknown>>(
        "/proposal/apply",
        { ...proposal, idempotencyKey: proposal.id },
      );
      await getStore().saveProposalReceipt(
        proposal.id,
        principal.actor,
        receipt,
      );
      return json({ ...proposal, receipt });
    }
  }
  if (path === "/api/documents" && request.method === "GET") {
    const live = url.searchParams.get("live");
    if (live)
      return json(await callBridge("/document/read", { documentId: live }));
    return json(
      (await getSnapshot()).documents.map(({ id, title, revision, path }) => ({
        id,
        title,
        revision,
        editable: path.startsWith("content/") && path.endsWith(".md"),
      })),
    );
  }
  if (path === "/api/runs" && request.method === "GET")
    return json(await getStore().listRuns(principal.actor));
  if (path === "/api/runs" && request.method === "POST") {
    const body = await readBody(request);
    const snapshot = await getSnapshot();
    const workflowId = stringField(body, "workflowId");
    const doc = snapshot.documents.find(
      (d) => d.id === `workflow-${workflowId}`,
    );
    if (!doc) throw new AccessError("workflow_not_found", 404);
    const project = stringField(body, "project");
    if (
      !["workspace", "frontend", "backend", "agent", "canvas"].includes(project)
    )
      throw new AccessError("unknown_project", 400);
    const selected = Array.isArray(body.documents)
      ? body.documents
          .filter((id): id is string => typeof id === "string")
          .slice(0, 24)
      : [];
    const required = [
      ...new Set([
        ...selected,
        "root-agents",
        "root-project",
        `workflow-${workflowId}`,
      ]),
    ];
    const live = await callBridge<LiveState>("/context", {
      project,
      workflowId,
      documents: required,
    });
    const actualWorkflow = parseWorkflow(live.workflowSource);
    const allowedActions =
      body.allowWrite === true
        ? ["read", "write", "verify"]
        : ["read", "verify"];
    const context = buildContext({
      goal: stringField(body, "goal", 8000),
      project,
      repoCommit: live.repoCommit,
      codeDigest: live.codeDigest,
      documents: required.flatMap((id) =>
        live.documents.filter((d) => d.id === id),
      ),
      skillRevisions: live.skillRevisions,
      workflowRevision: live.workflowRevision,
      allowedActions,
      expiresAt: Math.min(principal.expiresAt, Date.now() + 30 * 60 * 1000),
      acceptance: [stringField(body, "acceptance", 4000)],
      stageIds: actualWorkflow.stages.map((s) => s.id),
    });
    return json(
      await getStore().createRun(
        principal.actor,
        workflowId,
        context,
        stringField(body, "idempotencyKey"),
      ),
      201,
    );
  }
  const runId = path.match(
    /^\/api\/runs\/([a-f0-9-]{36})(?:\/(events|action|context))?$/,
  );
  if (runId) {
    const run = await getStore().getRun(runId[1], principal.actor);
    if (!run) throw new AccessError("run_not_found", 404);
    if (request.method === "GET") {
      if (runId[2] === "events")
        return json(await getStore().events(run.id, principal.actor));
      if (runId[2] === "context") return json(run.context);
      return json(run);
    }
    if (request.method === "POST" && runId[2] === "action") {
      const body = await readBody(request);
      const action = stringField(body, "action");
      const key = stringField(body, "idempotencyKey");
      const version = Number(body.version);
      if (!Number.isSafeInteger(version))
        throw new AccessError("invalid_version", 400);
      const current = ["pause", "cancel", "reconcile", "refresh"].includes(
        action,
      )
        ? { current: true }
        : await callBridge<{ current: boolean }>("/context/check", {
            context: run.context,
            workflowId: run.workflowId,
          });
      if (action === "start") {
        const claim = await getStore().transition(
          run.id,
          principal.actor,
          version,
          key,
          { type: "start" },
          current.current,
        );
        if (claim.replayed) return json(claim.run);
        try {
          const result = await callBridge<BridgeTask>("/run/start", {
            runId: run.id,
            attempt: claim.run.attempt,
            context: run.context,
            workflowId: run.workflowId,
            stage: run.stage,
          });
          return json(
            await settleExternal(
              (await getStore().getRun(run.id, principal.actor)) ?? claim.run,
              result,
              key + ":dispatch",
            ),
          );
        } catch {
          const now = await getStore().getRun(run.id, principal.actor);
          if (now?.status === "canceled") {
            await callBridge("/run/interrupt", {
              runId: run.id,
              attempt: claim.run.attempt,
            }).catch(() => undefined);
            return json(now);
          }
          if (now?.status === "running")
            return json(
              (
                await getStore().transition(
                  run.id,
                  principal.actor,
                  now.version,
                  key + ":unknown",
                  { type: "unknown" },
                  true,
                )
              ).run,
            );
          throw new AccessError("execution_unknown", 503);
        }
      }
      if (action === "reconcile") {
        if (run.status === "canceled" && run.attempt > 0) {
          await callBridge("/run/interrupt", {
            runId: run.id,
            attempt: run.attempt,
          });
          return json(run);
        }
        if (run.pendingCheck) {
          const evidence = await callBridge<Evidence>("/check/query", {
            runId: run.id,
            idempotencyKey: run.pendingCheck.key,
          });
          const changed = await getStore().transition(
            run.id,
            principal.actor,
            version,
            key,
            { type: "evidence", evidence },
            true,
          );
          await getStore().recordArtifact(changed.run, evidence);
          return json(changed.run);
        }
        return json(
          await settleExternal(
            run,
            await callBridge<BridgeTask>("/run/query", {
              runId: run.id,
              attempt: run.attempt,
            }),
            key,
          ),
        );
      }
      if (action === "refresh") {
        const live = await callBridge<LiveState>("/context", {
          project: run.context.project,
          workflowId: run.workflowId,
          documents: run.context.documents.map((d) => d.id),
        });
        if (live.workflowRevision !== run.context.workflowRevision)
          throw new AccessError("workflow_changed_new_run_required", 409);
        const context = buildContext({
          goal: run.context.goal,
          project: run.context.project,
          repoCommit: live.repoCommit,
          codeDigest: live.codeDigest,
          documents: live.documents,
          skillRevisions: live.skillRevisions,
          workflowRevision: live.workflowRevision,
          allowedActions: run.context.allowedActions,
          expiresAt: principal.expiresAt,
          acceptance: run.context.acceptance,
          stageIds: run.context.stageIds,
        });
        return json(
          (
            await getStore().transition(
              run.id,
              principal.actor,
              version,
              key,
              { type: "refresh", context },
              true,
            )
          ).run,
        );
      }
      if (action === "verify") {
        if (!current.current) throw new AccessError("inputs_stale", 409);
        const commandId = stringField(body, "commandId");
        try {
          verificationCommand(run.context.project, commandId);
        } catch (error) {
          throw new AccessError((error as Error).message, 400);
        }
        const claim = await getStore().transition(
          run.id,
          principal.actor,
          version,
          key,
          { type: "verifyStart", key, commandId },
          current.current,
        );
        if (claim.replayed) return json(claim.run);
        const evidence = await callBridge<
          Evidence | { notDispatched: true; reason: string }
        >("/check", {
          runId: run.id,
          context: run.context,
          workflowId: run.workflowId,
          commandId,
          idempotencyKey: key,
        });
        if ("notDispatched" in evidence) {
          return json(
            (
              await getStore().transition(
                run.id,
                principal.actor,
                claim.run.version,
                key + ":not-dispatched",
                { type: "checkRejected" },
                true,
              )
            ).run,
          );
        }
        const changed = await getStore().transition(
          run.id,
          principal.actor,
          claim.run.version,
          key + ":result",
          { type: "evidence", evidence },
          true,
        );
        await getStore().recordArtifact(changed.run, evidence);
        return json(changed.run);
      }
      let change: Action;
      if (action === "accept") {
        requireAccess(request, "accept", true);
        change = { type: "accept", actor: principal.actor };
      } else if (action === "advance") {
        change = { type: "advance", stages: run.context.stageIds.length };
      } else if (["pause", "resume", "cancel"].includes(action))
        change = { type: action as "pause" | "resume" | "cancel" };
      else throw new AccessError("unknown_action", 400);
      const result = await getStore().transition(
        run.id,
        principal.actor,
        version,
        key,
        change,
        current.current,
      );
      if (action === "cancel" && run.attempt > 0)
        await callBridge("/run/interrupt", {
          runId: run.id,
          attempt: run.attempt,
        }).catch(() => undefined);
      return json(result.run);
    }
  }
  if (path === "/api/narration/generate")
    throw new AccessError("tts_disabled_by_owner", 403);
  throw new AccessError("not_found", 404);
}
async function settleExternal(run: Run, result: BridgeTask, key: string) {
  if (run.status === "canceled" && run.attempt > 0) {
    await callBridge("/run/interrupt", {
      runId: run.id,
      attempt: run.attempt,
    }).catch(() => undefined);
    return run;
  }
  if (!["running", "waiting_external", "unknown"].includes(run.status))
    return run;
  if (
    result.status === "running" &&
    result.handle &&
    run.externalHandle !== result.handle
  )
    return (
      await getStore().transition(
        run.id,
        run.actor,
        run.version,
        key,
        { type: "bind", handle: result.handle },
        true,
      )
    ).run;
  if (result.status === "completed" && result.handle)
    return (
      await getStore().transition(
        run.id,
        run.actor,
        run.version,
        key,
        {
          type: "external",
          handle: result.handle,
          result: result.result ?? "",
        },
        true,
      )
    ).run;
  if (result.status === "failed")
    return (
      await getStore().transition(
        run.id,
        run.actor,
        run.version,
        key,
        { type: "fail" },
        true,
      )
    ).run;
  if (
    result.status === "unknown" &&
    !["canceled", "succeeded", "paused"].includes(run.status)
  )
    return (
      await getStore().transition(
        run.id,
        run.actor,
        run.version,
        key,
        { type: "unknown" },
        true,
      )
    ).run;
  return run;
}
export async function GET(request: Request) {
  try {
    return await handle(request);
  } catch (error) {
    return errorResponse(error);
  }
}
export async function POST(request: Request) {
  try {
    return await handle(request);
  } catch (error) {
    return errorResponse(error);
  }
}
