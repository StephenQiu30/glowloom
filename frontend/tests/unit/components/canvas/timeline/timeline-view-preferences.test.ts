import { afterEach, expect, it, vi } from "vitest";
import {
  setSkipJianyingReminder,
  skipJianyingReminder,
} from "@/components/canvas/timeline/timeline-view-preferences";
const key = "glowloom:timeline:jianying:skip-reminder";
afterEach(() => {
  vi.restoreAllMocks();
  localStorage.removeItem(key);
});
it("accepts only the exact optional boolean marker and removes it when reminders are restored", () => {
  expect(skipJianyingReminder()).toBe(false);
  for (const invalid of ['{"skip":true}', "1", "TRUE", "false", ""]) {
    localStorage.setItem(key, invalid);
    expect(skipJianyingReminder()).toBe(false);
  }
  const writes = vi.spyOn(Storage.prototype, "setItem");
  setSkipJianyingReminder(true);
  expect(writes).toHaveBeenCalledExactlyOnceWith(key, "true");
  expect(skipJianyingReminder()).toBe(true);
  setSkipJianyingReminder(false);
  expect(localStorage.getItem(key)).toBeNull();
  expect(skipJianyingReminder()).toBe(false);
});
it("keeps export available when browser storage is blocked", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  expect(skipJianyingReminder()).toBe(false);
  expect(() => setSkipJianyingReminder(true)).not.toThrow();
  expect(() => setSkipJianyingReminder(false)).not.toThrow();
});
