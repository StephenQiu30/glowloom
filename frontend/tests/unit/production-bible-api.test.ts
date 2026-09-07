import { productionBibleApi } from "@/features/production-bible/endpoints";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/request", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/request")>(),
  default: requestMock,
}));

import { decideProductionBibleReviewIssue } from "@/api/productionBibles";
import { makeStore } from "@/lib/redux-store";
import { ApiClientError } from "@/lib/request";
import { appApi } from "@/lib/server-state";

const bibleId = "00000000-0000-0000-0000-000000000001";
const projectId = "00000000-0000-0000-0000-000000000002";
const body = {
  issue_key: "issue-1",
  action: "accepted" as const,
  expected_revision: 2,
  idempotency_key: "review:issue-1",
};

describe("Production Bible review contract", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
    requestMock.mockReset();
  });

  afterEach(() => {
    store.dispatch(appApi.util.resetApiState());
  });

  it("sends the exact decision and propagates transport options and failures", async () => {
    const controller = new AbortController();
    requestMock.mockResolvedValueOnce({ data: { id: bibleId, revision: 3 } });

    await expect(
      decideProductionBibleReviewIssue({ bible_id: bibleId }, body, {
        signal: controller.signal,
      }),
    ).resolves.toEqual({ data: { id: bibleId, revision: 3 } });
    expect(requestMock).toHaveBeenCalledWith(
      `/api/production-bibles/${bibleId}/review-decisions`,
      { method: "POST", data: body, signal: controller.signal },
    );

    const failure = new ApiClientError("版本已变化", "resource_conflict", "refresh");
    requestMock.mockRejectedValueOnce(failure);
    await expect(
      decideProductionBibleReviewIssue({ bible_id: bibleId }, body),
    ).rejects.toBe(failure);
  });

  it("refreshes project and Bible queries after a persisted decision", async () => {
    let revision = 2;
    requestMock.mockImplementation(async (_url: string, options?: { method?: string }) => {
      if (options?.method === "POST") revision = 3;
      return { data: { id: bibleId, revision, review_decisions: revision === 3 ? { "issue-1": "accepted" } : {} } };
    });

    const current = store.dispatch(productionBibleApi.endpoints.currentProductionBible.initiate(projectId));
    const detail = store.dispatch(productionBibleApi.endpoints.productionBible.initiate(bibleId));
    await Promise.all([current.unwrap(), detail.unwrap()]);

    await expect(store.dispatch(productionBibleApi.endpoints.decideProductionBibleReviewIssue.initiate({
      projectId, bibleId, body,
    })).unwrap()).resolves.toMatchObject({ revision: 3 });

    await vi.waitFor(() => {
      expect(productionBibleApi.endpoints.currentProductionBible.select(projectId)(store.getState()).data?.revision).toBe(3);
      expect(productionBibleApi.endpoints.productionBible.select(bibleId)(store.getState()).data?.review_decisions).toEqual({ "issue-1": "accepted" });
    });
    expect(requestMock).toHaveBeenCalledTimes(5);
    current.unsubscribe();
    detail.unsubscribe();
  });

  it("keeps conflict details and never returns success for a rejected command", async () => {
    requestMock.mockRejectedValueOnce(new ApiClientError(
      "版本已变化", "resource_conflict", "refresh", { expected_revision: 3 },
    ));
    await expect(store.dispatch(productionBibleApi.endpoints.decideProductionBibleReviewIssue.initiate({
      projectId, bibleId, body,
    })).unwrap()).rejects.toEqual({
      message: "版本已变化", code: "resource_conflict", nextAction: "refresh",
      details: { expected_revision: 3 },
    });
    expect(requestMock).toHaveBeenCalledTimes(1);
  });
});
