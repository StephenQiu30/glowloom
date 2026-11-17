import { describe, expect, it } from "vitest";
import {
  beijingDateTime,
  beijingInstant,
} from "@/components/notifications/notification-time";

describe("公告北京时间", () => {
  it("将北京时间输入转换为绝对时刻，不依赖浏览器时区", () => {
    expect(beijingInstant("2026-11-17T09:00")).toBe("2026-11-17T01:00:00.000Z");
    expect(beijingDateTime("2026-11-17T01:00:00.000Z")).toBe(
      "2026-11-17T09:00",
    );
    expect(beijingDateTime("2026-11-16T16:00:00.000Z")).toBe(
      "2026-11-17T00:00",
    );
  });
  it("拒绝不完整或不存在的日期，避免静默顺延发布", () => {
    expect(beijingInstant("")).toBe("");
    expect(beijingInstant("2026-02-30T09:00")).toBe("");
    expect(beijingInstant("2026-11-17")).toBe("");
    expect(beijingDateTime("invalid")).toBe("");
  });
  it("接受有效闰日", () => {
    expect(beijingInstant("2028-02-29T00:00")).toBe("2028-02-28T16:00:00.000Z");
  });
});
