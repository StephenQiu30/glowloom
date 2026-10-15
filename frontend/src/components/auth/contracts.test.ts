import { expect, it } from "vitest";
import { requireAccepted } from "./contracts";
const valid = {
  operation_id: "33333333-3333-4333-8333-333333333333",
  retry_after_seconds: 60,
};
it("mail acceptance requires a real operation UUID and bounded integer countdown", () => {
  expect(requireAccepted(valid)).toEqual(valid);
  for (const value of [
    null,
    {},
    { ...valid, operation_id: "wrong" },
    { ...valid, operation_id: "00000000-0000-0000-0000-000000000000" },
    { ...valid, retry_after_seconds: -1 },
    { ...valid, retry_after_seconds: Infinity },
    { ...valid, retry_after_seconds: 1.5 },
    { ...valid, retry_after_seconds: 86401 },
  ])
    expect(() => requireAccepted(value)).toThrow("invalid_receipt");
});
