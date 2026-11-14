import { expect, it } from "vitest";
import { verificationCommand } from "../../src/server/bridge/verification";
it("maps only known checks to existing engineering contracts", () => {
  for (const p of ["workspace", "frontend", "backend", "agent", "canvas"])
    expect(verificationCommand(p, "typecheck").args.length).toBeGreaterThan(0);
  expect(() => verificationCommand("frontend", "unit")).toThrow(/configured/);
  expect(() => verificationCommand("workspace", "typo")).toThrow(/command/);
});
