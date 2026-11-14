import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";
test.use({
  baseURL: process.env.WORKSPACE_ONLINE_TEST_URL ?? "http://127.0.0.1:1130",
});
const token =
  process.env.WORKSPACE_ONLINE_TEST_TOKEN ??
  "fixture-online-owner-token-local-tests-only-20261114";
const origin = process.env.WORKSPACE_ONLINE_TEST_URL ?? "http://127.0.0.1:1130";
test("keeps unauthenticated login free of private knowledge and protects every data resource", async ({
  page,
}) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "访问工作空间" }),
  ).toBeVisible();
  expect(await page.content()).not.toContain(
    "Workspace 知识库与研发工作流服务设计",
  );
  for (const path of [
    "/raw/design-43.md",
    "/pagefind/pagefind.js",
    "/narration/index.json",
    "/api/documents/",
  ])
    expect((await page.request.get(path)).status()).toBe(401);
});
test("authenticates local ownership, rejects cross-origin writes, and preserves conflicting proposals", async ({
  page,
}) => {
  await page.goto("/login/");
  await page.getByLabel("本机访问令牌").fill(token);
  await page.getByRole("button", { name: "进入工作空间" }).click();
  await expect(
    page.getByRole("heading", { name: "知识库", exact: true }),
  ).toBeVisible();
  const session = await (await page.request.get("/api/session/")).json();
  const headers = { origin, "x-workspace-csrf": session.csrf };
  const status = await (await page.request.get("/api/status/")).json();
  expect(status.database).toBe(true);
  expect(status.bridge).toBe(true);
  expect(status.tts).toBe(false);
  expect(
    (
      await page.request.post("/api/proposals/", {
        headers: { ...headers, origin: "https://evil.example" },
        data: {},
      })
    ).status(),
  ).toBe(403);
  const doc = await (
    await page.request.get("/api/documents?live=requirement-28")
  ).json();
  let modified = false;
  const create = async (text: string, base = doc) => {
    const r = await page.request.post("/api/proposals/", {
      headers,
      data: {
        documentId: base.id,
        baseRevision: base.revision,
        baseText: base.source,
        text,
        idempotencyKey: randomUUID(),
      },
    });
    expect(r.status()).toBe(201);
    return r.json();
  };
  try {
    const a = await create(doc.source + "\n<!-- workspace-e2e-owned -->\n");
    const b = await create(doc.source + "\n<!-- conflicting -->\n");
    const applied = await page.request.post(`/api/proposals/${a.id}/apply/`, {
      headers,
      data: {},
    });
    expect(applied.status()).toBe(200);
    modified = true;
    const receipt = (await applied.json()).receipt;
    expect(receipt.status).toBe("applied");
    const conflict = await page.request.post(`/api/proposals/${b.id}/apply/`, {
      headers,
      data: {},
    });
    expect(conflict.status()).toBe(409);
    const replay = await page.request.post(`/api/proposals/${a.id}/apply/`, {
      headers,
      data: {},
    });
    expect((await replay.json()).receipt).toEqual(receipt);
    expect(
      (await (await page.request.get(`/api/proposals/${b.id}/`)).json())
        .receipt,
    ).toBeNull();
  } finally {
    if (modified) {
      const current = await (
        await page.request.get("/api/documents?live=requirement-28")
      ).json();
      const restore = await create(doc.source, current);
      expect(
        (
          await page.request.post(`/api/proposals/${restore.id}/apply/`, {
            headers,
            data: {},
          })
        ).status(),
      ).toBe(200);
    }
  }
  await page.goto("/operations/");
  await expect(page.getByRole("heading", { name: "在线协作台" })).toBeVisible();
  expect(
    (
      await page.request.post("/api/narration/generate/", { headers, data: {} })
    ).status(),
  ).toBe(403);
});
test("creates a frozen task context and verifies actual host discovery without dispatch", async ({
  page,
}) => {
  await page.goto("/login/");
  await page.getByLabel("本机访问令牌").fill(token);
  await page.getByRole("button", { name: "进入工作空间" }).click();
  await expect(
    page.getByRole("heading", { name: "知识库", exact: true }),
  ).toBeVisible();
  const session = await (await page.request.get("/api/session/")).json();
  const headers = { origin, "x-workspace-csrf": session.csrf };
  const created = await page.request.post("/api/runs/", {
    headers,
    data: {
      workflowId: "feature",
      project: "workspace",
      goal: "只读核对知识与工程入口",
      acceptance: "列出实际目录与原文版本",
      documents: ["requirement-28"],
      idempotencyKey: randomUUID(),
    },
  });
  expect(created.status()).toBe(201);
  const run = await created.json();
  expect(run.status).toBe("created");
  expect(run.context.allowedActions).not.toContain("write");
  expect(run.context.stageIds).toContain("accept");
  expect(
    run.context.documents.some((d: { id: string }) => d.id === "root-agents"),
  ).toBe(true);
  const skills = await page.request.post("/api/host/skills/", {
    headers,
    data: { project: "workspace" },
  });
  expect(skills.status()).toBe(200);
  expect(
    (await skills.json()).filter((s: { enabled: boolean }) => s.enabled),
  ).toHaveLength(4);
  const invalid = await page.request.post(`/api/runs/${run.id}/action/`, {
    headers,
    data: {
      action: "verify",
      commandId: "typo",
      version: run.version,
      idempotencyKey: randomUUID(),
    },
  });
  expect(invalid.status()).toBe(400);
  const unchanged = await (
    await page.request.get(`/api/runs/${run.id}/`)
  ).json();
  expect(unchanged.status).toBe("created");
  expect(unchanged.pendingCheck ?? null).toBeNull();
  const canceled = await page.request.post(`/api/runs/${run.id}/action/`, {
    headers,
    data: {
      action: "cancel",
      version: run.version,
      idempotencyKey: randomUUID(),
    },
  });
  expect(canceled.status()).toBe(200);
  expect((await canceled.json()).status).toBe("canceled");
});
