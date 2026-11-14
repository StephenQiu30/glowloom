import { execFileSync } from "node:child_process";
import { expect, it } from "vitest";
import {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  rm,
  chmod,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import { request } from "node:http";
import type { liveContext } from "../../src/server/bridge/context";
import { startBridge } from "../../src/server/bridge/service";
import { buildContext } from "../../src/workflows/definition";
it("remembers cancellation before a native turn handle is available", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-cancel-")),
    socket = "/tmp/ws-" + randomUUID() + ".sock",
    token = "fixture-token-with-at-least-thirty-two-chars";
  await mkdir(join(root, "workspace/workflows"), { recursive: true });
  await writeFile(
    join(root, "workspace/projects.json"),
    JSON.stringify([{ id: "workspace", path: "workspace" }]),
  );
  await writeFile(
    join(root, "workspace/catalog.json"),
    JSON.stringify([
      {
        id: "workflow-feature",
        path: "workflows/feature.md",
        visibility: "internal",
      },
    ]),
  );
  await writeFile(
    join(root, "workspace/workflows/feature.md"),
    "# Feature\n```workspace-workflow\n" +
      JSON.stringify({
        id: "feature",
        stages: [
          {
            id: "accept",
            label: "Accept",
            needs: [],
            outputs: ["result"],
            optional: false,
          },
        ],
      }) +
      "\n```",
  );
  const events = join(root, "events");
  const fake = join(root, "codex");
  await writeFile(
    fake,
    `#!/usr/bin/env node\nconst{createInterface}=require('node:readline');const{appendFileSync}=require('node:fs');createInterface({input:process.stdin}).on('line',line=>{const m=JSON.parse(line);if(m.id===undefined)return;appendFileSync(${JSON.stringify(events)},m.method+'\\n');let result={};if(m.method==='thread/start')result={thread:{id:'thread'}};if(m.method==='turn/start')result={turn:{id:'turn'}};if(m.method==='thread/read')result={thread:{turns:[{id:'turn',status:'inProgress',items:[]}]}};setTimeout(()=>process.stdout.write(JSON.stringify({id:m.id,result})+'\\n'),m.method==='thread/start'?150:0);});`,
  );
  await chmod(fake, 0o700);
  execFileSync("git", ["init", "-q"], { cwd: root });
  const bridge = await startBridge({
    repo: root,
    storage: join(root, "runtime"),
    socket,
    token,
    codexBinary: fake,
  });
  const call = <T = unknown>(path: string, body: unknown) =>
    new Promise<T>((yes, no) => {
      const req = request(
        {
          socketPath: socket,
          path,
          method: "POST",
          headers: { authorization: "Bearer " + token },
        },
        (res) => {
          let text = "";
          res.on("data", (c) => (text += c));
          res.on("end", () => yes(JSON.parse(text)));
        },
      );
      req.on("error", no);
      req.end(JSON.stringify(body));
    });
  try {
    const live = await call<Awaited<ReturnType<typeof liveContext>>>(
      "/context",
      {
        project: "workspace",
        workflowId: "feature",
      },
    );
    const context = buildContext({
      goal: "fixture",
      project: "workspace",
      repoCommit: live.repoCommit,
      codeDigest: live.codeDigest,
      documents: [],
      skillRevisions: live.skillRevisions,
      workflowRevision: live.workflowRevision,
      allowedActions: ["read"],
      acceptance: ["fixture"],
      expiresAt: Date.now() + 60000,
    });
    const runId = randomUUID();
    const started = call("/run/start", {
      runId,
      attempt: 1,
      context,
      workflowId: "feature",
      stage: 0,
    });
    await new Promise((r) => setTimeout(r, 50));
    await call("/run/interrupt", { runId, attempt: 1 });
    await started;
    const log = await readFile(events, "utf8");
    expect(!log.includes("turn/start") || log.includes("turn/interrupt")).toBe(
      true,
    );
  } finally {
    await bridge.close();
    await rm(root, { recursive: true, force: true });
  }
});
