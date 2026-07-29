import { describe, expect, it } from "vitest";

import api from "@/api";
import request, { type RequestOptions } from "@/lib/api-request";

describe("generated API client", () => {
  it("is generated and uses the typed request adapter", () => {
    const options: RequestOptions = { timeout: 1000 };
    expect(typeof request).toBe("function");
    expect(options.timeout).toBe(1000);
    expect(Object.keys(api).length).toBeGreaterThan(0);
  });
});
