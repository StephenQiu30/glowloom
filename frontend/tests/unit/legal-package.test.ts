import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { expect, it } from "vitest";
it("public downloads, reviewed source and server registration package have identical immutable hashes", async () => {
  const server = JSON.parse(
    await readFile(
      "../backend/internal/identity/adapter/http/legal/manifest.json",
      "utf8",
    ),
  );
  const browser = JSON.parse(
    await readFile(`public/legal/${server.version}/manifest.json`, "utf8"),
  );
  expect(browser).toEqual(server);
  expect(server.status).toBe("prepared-not-effective");
  for (const doc of server.documents) {
    const text = await readFile(`../${doc.source}`, "utf8");
    const download = await readFile(
      `public/legal/${server.version}/${doc.path.slice(1)}.md`,
      "utf8",
    );
    expect(text).toBe(doc.text);
    expect(download).toBe(doc.text);
    expect(createHash("sha256").update(text).digest("hex")).toBe(doc.sha256);
  }
});
