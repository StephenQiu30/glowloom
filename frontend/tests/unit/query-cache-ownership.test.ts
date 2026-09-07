import { identityApi } from "@/features/identity/endpoints";
import { projectApi } from "@/features/project/endpoints";
import { afterEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/request", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/request")>(),
  default: requestMock,
}));

import { makeStore } from "@/lib/redux-store";
import { appApi } from "@/lib/server-state";

describe("shared Query cache", () => {
  const store = makeStore();
  afterEach(() => {
    store.dispatch(appApi.util.resetApiState());
    requestMock.mockReset();
  });

  it("deduplicates shared identity reads and clears all feature data together", async () => {
    requestMock.mockImplementation(async (url: string) => ({ data: { id: url } }));
    const first = store.dispatch(identityApi.endpoints.me.initiate());
    const second = store.dispatch(identityApi.endpoints.me.initiate());
    const project = store.dispatch(projectApi.endpoints.project.initiate("project-1"));
    await Promise.all([first.unwrap(), second.unwrap(), project.unwrap()]);
    expect(requestMock.mock.calls.filter(([url]) => url === "/api/me")).toHaveLength(1);
    expect(Object.keys(store.getState())).toEqual(["appApi"]);
    first.unsubscribe();
    second.unsubscribe();
    project.unsubscribe();

    store.dispatch(appApi.util.resetApiState());
    expect(identityApi.endpoints.me.select()(store.getState()).data).toBeUndefined();
    expect(projectApi.endpoints.project.select("project-1")(store.getState()).data).toBeUndefined();
  });
});
