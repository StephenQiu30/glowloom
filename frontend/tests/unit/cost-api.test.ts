import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/request", () => ({ default: requestMock }));

import {
  getCurrentCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPriceGet,
  setCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPricePost,
} from "@/api/cost";

const projectId = "019ffb00-a000-7000-8000-000000000001";
const profileId = "019ffb00-a000-7000-8000-000000000002";
const exactPath =
  `/api/projects/${projectId}/media-model-profiles/${profileId}/cost-price`;

describe("exact ModelProfile Cost API", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    requestMock.mockResolvedValue({ data: {} });
  });

  it("reads and writes price by exact profile version without a metric route", async () => {
    const body: API.CostPriceQuoteSetRequest = {
      reservation_unit_amount: "0.125000",
      currency: "USD",
      expected_revision: 0,
      idempotency_key: "exact-price:create",
    };

    await getCurrentCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPriceGet({
      project_id: projectId,
      profile_version_id: profileId,
    });
    await setCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPricePost(
      { project_id: projectId, profile_version_id: profileId },
      body,
    );

    expect(requestMock).toHaveBeenNthCalledWith(1, exactPath, { method: "GET" });
    expect(requestMock).toHaveBeenNthCalledWith(2, exactPath, {
      method: "POST",
      data: body,
    });
    expect(JSON.stringify(requestMock.mock.calls)).not.toContain("cost-prices");
    expect(body).not.toHaveProperty("unit_amount");
  });
});
