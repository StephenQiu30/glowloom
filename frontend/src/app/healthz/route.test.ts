import { describe, expect, it } from "vitest";

import { GET } from "./route";

describe("GET /healthz", () => {
  it("reports ok", async () => {
    const response = GET();

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ status: "ok" });
  });
});
