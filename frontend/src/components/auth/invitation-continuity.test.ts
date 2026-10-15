import { afterEach, expect, it } from "vitest";
import {
  clearInvitation,
  pendingInvitation,
  readInvitationFragment,
} from "./invitation-continuity";
afterEach(() => {
  clearInvitation();
  window.history.replaceState(null, "", "/");
});
it("StrictMode 重读不丢失内存邀请，清除地址栏令牌且拒绝重复参数", () => {
  const proof = "a".repeat(48);
  window.history.replaceState(null, "", `/invitations#token=${proof}`);
  expect(readInvitationFragment()).toBe(proof);
  expect(window.location.hash).toBe("");
  expect(readInvitationFragment()).toBe(proof);
  expect(pendingInvitation()).toBe(proof);
  window.history.replaceState(null, "", "/invitations#token=one&token=two");
  expect(readInvitationFragment()).toBe("");
});
