import { describe, expect, it } from "vitest";

import {
  canAccessPage,
  visiblePrimaryNavigation,
} from "@/lib/access-control";

describe("workspace page access", () => {
  it("does not expose protected navigation before authentication", () => {
    expect(visiblePrimaryNavigation(undefined)).toEqual(["create"]);
    expect(canAccessPage(undefined, "projects")).toBe(false);
    expect(canAccessPage(undefined, "settings")).toBe(false);
  });

  it("keeps viewer navigation read-only and excludes governance", () => {
    expect(visiblePrimaryNavigation("viewer")).toEqual(["projects", "assets"]);
    expect(canAccessPage("viewer", "projects")).toBe(true);
    expect(canAccessPage("viewer", "assets")).toBe(true);
    expect(canAccessPage("viewer", "governance")).toBe(false);
    expect(canAccessPage("viewer", "settings")).toBe(true);
  });

  it.each(["editor", "owner"] as const)(
    "allows %s to enter governance without promoting it to primary navigation",
    (role) => {
      expect(visiblePrimaryNavigation(role)).toEqual(["projects", "assets"]);
      expect(canAccessPage(role, "governance")).toBe(true);
    },
  );
});
