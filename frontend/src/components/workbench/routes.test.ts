import { expect, it } from "vitest";
import { resolveProjectRoute, updatePreviewQuery } from "./routes";

it("resolves project, episode and shot deep links while rejecting unknown objects", () => {
  expect(
    resolveProjectRoute("harbor", ["episodes", "ep-01", "shots", "shot-01"])
      ?.kind,
  ).toBe("shot");
  expect(
    resolveProjectRoute("harbor", ["episodes", "ep-01", "storyboard"])?.kind,
  ).toBe("storyboard");
  expect(
    resolveProjectRoute("harbor", ["episodes", "missing", "shots"]),
  ).toBeNull();
  expect(
    resolveProjectRoute("harbor", ["episodes", "ep-01", "shots", "missing"]),
  ).toBeNull();
  expect(resolveProjectRoute("missing", [])).toBeNull();
  expect(resolveProjectRoute("harbor", ["unknown"])).toBeNull();
});

it("updates one URL filter without losing the selected candidate or page state", () => {
  expect(updatePreviewQuery("candidate=b&state=readonly", "q", "港口")).toBe(
    "candidate=b&state=readonly&q=%E6%B8%AF%E5%8F%A3",
  );
  expect(updatePreviewQuery("q=hello&view=table", "q", "")).toBe("view=table");
});
