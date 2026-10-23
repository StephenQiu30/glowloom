// @vitest-environment node
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, beforeAll, expect, it } from "vitest";
import { getGenerationOptions } from "@/components/canvas/runtime/api/generationBffController";
import { unwrap } from "@/components/canvas/runtime/lib/request";

const projectId = "deca74c9-403f-4fb2-afb3-ac3a58048bb8";
let server: Server;
let baseURL: string;
let received: URL;
beforeAll(async () => {
  server = createServer((request, response) => {
    received = new URL(request.url!, "http://localhost");
    response.setHeader("Content-Type", "application/json");
    if (
      received.pathname !== "/api/models" ||
      received.searchParams.get("project_id") !== projectId ||
      received.searchParams.get("capability") !== "image.generate" ||
      received.searchParams.has("modality")
    ) {
      response.writeHead(422).end(JSON.stringify({ code: "invalid_request" }));
      return;
    }
    response.end(
      JSON.stringify({
        items: [
          {
            current_version: {
              param_schema: [
                { key: "count", type: "integer", enum: [1, 2], default: 1 },
              ],
            },
          },
        ],
      }),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  baseURL = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
});

it("translates canvas option queries without overwriting the Go model scope", async () => {
  const result = unwrap(
    await getGenerationOptions(
      { modality: "image" },
      { baseURL, params: { modality: "image", project_id: projectId } },
    ),
  );
  expect(received.searchParams.get("limit")).toBe("200");
  expect(result.parameters).toEqual([
    {
      key: "count",
      label: "count",
      numeric: true,
      values: ["1", "2"],
      defaultValue: "1",
    },
  ]);
});
