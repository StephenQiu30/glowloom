import { expect, it } from "vitest";
import { aliasRedirect } from "../../src/build/aliases";
it("preserves hashes in old knowledge URLs and rejects path escapes", () => {
  expect(aliasRedirect("docs/design/43-A.md", "design-43", "")).toContain(
    "/read/design-43/",
  );
  expect(aliasRedirect("docs/design/43-A.md", "design-43", "")).toContain(
    "location.hash",
  );
  expect(() => aliasRedirect("../secret", "a", "")).toThrow(/alias/);
});
