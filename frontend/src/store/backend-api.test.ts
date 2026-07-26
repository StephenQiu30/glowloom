import { describe, expect, it, vi } from "vitest";

import { confirmSource } from "@/api/confirmSource";
import { createProject } from "@/api/createProject";
import { listProjects } from "@/api/listProjects";
import { backendApi } from "@/store/backend-api";
import { makeStore } from "@/store/make-store";

vi.mock("@/api/confirmSource", () => ({ confirmSource: vi.fn() }));
vi.mock("@/api/createProject", () => ({ createProject: vi.fn() }));
vi.mock("@/api/listProjects", () => ({ listProjects: vi.fn() }));

describe("backendApi generated call boundary", () => {
  it("passes the user intent key to generated createProject", async () => {
    vi.mocked(createProject).mockResolvedValue({ project: { id: "project-1" } } as never);
    const store = makeStore();

    await store
      .dispatch(
        backendApi.endpoints.createProject.initiate({
          title: "雾中来信",
          idempotencyKey: "create-project:0001",
        }),
      )
      .unwrap();

    expect(createProject).toHaveBeenCalledWith(
      { title: "雾中来信" },
      { headers: { "Idempotency-Key": "create-project:0001" } },
    );
  });

  it("derives the strong If-Match header from generated resource_version", async () => {
    vi.mocked(confirmSource).mockResolvedValue({ id: "source-1" } as never);
    const store = makeStore();

    await store
      .dispatch(
        backendApi.endpoints.confirmSource.initiate({ versionId: "source-1", resourceVersion: 7 }),
      )
      .unwrap();

    expect(confirmSource).toHaveBeenCalledWith(
      { version_id: "source-1" },
      { headers: { "If-Match": '"7"' } },
    );
  });

  it("normalizes network failures into serializable Redux errors", async () => {
    vi.mocked(listProjects).mockRejectedValue(new TypeError("Failed to fetch"));
    const store = makeStore();

    const result = await store.dispatch(backendApi.endpoints.listProjects.initiate());

    expect(result).toMatchObject({
      error: {
        status: "FETCH_ERROR",
        data: { code: "NETWORK_ERROR", detail: "Failed to fetch" },
      },
    });
    expect(() => JSON.stringify(result)).not.toThrow();
  });

  it("preserves generated HTTP status for conflict handling", async () => {
    vi.mocked(confirmSource).mockRejectedValue({
      status: 412,
      problem: { code: "PRECONDITION_FAILED" },
    });
    const store = makeStore();

    const request = store.dispatch(
      backendApi.endpoints.confirmSource.initiate({ versionId: "source-1", resourceVersion: 7 }),
    );

    await expect(request.unwrap()).rejects.toMatchObject({
      status: 412,
      data: { code: "PRECONDITION_FAILED" },
    });
  });
});
