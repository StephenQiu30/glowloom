import { describe, expect, it } from "vitest";

import {
  summarizeFrames,
  measurementQuality,
  isMotionSample,
} from "./poc-measurement";

describe("画布交互测量", () => {
  it("按住鼠标但未移动时不持续统计空闲帧", () => {
    expect(isMotionSample(true, 100, 200)).toBe(true);
    expect(isMotionSample(true, 100, 1000)).toBe(false);
    expect(isMotionSample(false, 100, 110)).toBe(false);
    // RAF timestamps mark the frame start and can precede the input handler clock.
    expect(isMotionSample(true, 100.5, 100)).toBe(true);
  });
  it("只统计连续运动窗口，不将空闲或窗口间隔计入帧率", () => {
    const result = summarizeFrames([
      [0, 20, 40],
      [1000, 1010, 1020],
    ]);
    expect(result.frameIntervals).toBe(4);
    expect(result.activeMs).toBe(60);
    expect(result.meanFps).toBeCloseTo(66.67, 2);
    expect(result.p95FrameMs).toBe(20);
  });

  it("没有足够运动帧时不给出帧率", () => {
    expect(summarizeFrames([[], [1]]).meanFps).toBeNull();
    expect(summarizeFrames([]).p95FrameMs).toBeNull();
  });

  it("拒绝无效和倒序时间戳", () => {
    expect(() => summarizeFrames([[0, Number.NaN]])).toThrow();
    expect(() => summarizeFrames([[20, 10]])).toThrow();
  });

  it("短时冒烟、隐藏页面、程序输入不能算目标设备交互验收", () => {
    const base = {
      elapsedMs: 60000,
      activeMs: 45000,
      trustedInputs: 80,
      interrupted: false,
    };
    expect(measurementQuality(base)).toBe("有效本机交互记录");
    expect(measurementQuality({ ...base, elapsedMs: 10000 })).toBe(
      "短时冒烟记录",
    );
    expect(measurementQuality({ ...base, trustedInputs: 0 })).toBe(
      "缺少真实交互输入",
    );
    expect(measurementQuality({ ...base, activeMs: 100 })).toBe("运动采样不足");
    expect(measurementQuality({ ...base, interrupted: true })).toBe(
      "记录被中断",
    );
  });
});
