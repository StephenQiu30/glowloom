"use client";

import { useEffect, useRef, useState, type RefObject } from "react";

import { Button } from "@/components/ui/button";
import {
  isMotionSample,
  measurementQuality,
  summarizeFrames,
} from "./poc-measurement";

type Report = ReturnType<typeof summarizeFrames> & {
  schemaVersion: 1;
  recordedAt: string;
  elapsedMs: number;
  trustedInputs: number;
  interrupted: boolean;
  quality: string;
  scenario: { nodes: number; edges: number };
  media: { images: number; videos: number; bytes: number };
  device: {
    userAgent: string;
    viewport: number[];
    pixelRatio: number;
    hardwareConcurrency: number;
    jsHeapBytes: number | null;
  };
  renderedMedia: {
    images: number;
    loadedImages: number;
    videos: number;
    playingVideos: number;
    failedMedia: number;
  };
};

type Recording = {
  started: number;
  inputs: number;
  windows: number[][];
  current: number[] | null;
  animation: number;
  timeout: ReturnType<typeof setTimeout> | null;
};

export function PocRecorder({
  moving,
  lastMotion,
  inputs,
  region,
  nodes,
  edges,
  media,
}: {
  moving: RefObject<boolean>;
  lastMotion: RefObject<number>;
  inputs: RefObject<number>;
  region: RefObject<HTMLDivElement | null>;
  nodes: number;
  edges: number;
  media: { images: number; videos: number; bytes: number };
}) {
  const recording = useRef<Recording | null>(null);
  const [running, setRunning] = useState(false);
  const [report, setReport] = useState<Report | null>(null);

  function finish(interrupted: boolean) {
    const current = recording.current;
    if (!current) return;
    recording.current = null;
    cancelAnimationFrame(current.animation);
    if (current.timeout) clearTimeout(current.timeout);
    const stats = summarizeFrames(current.windows);
    const elapsedMs = performance.now() - current.started;
    const trustedInputs = inputs.current - current.inputs;
    const images = Array.from(region.current?.querySelectorAll("img") ?? []);
    const videos = Array.from(region.current?.querySelectorAll("video") ?? []);
    const memory = (
      performance as Performance & { memory?: { usedJSHeapSize: number } }
    ).memory;
    setReport({
      ...stats,
      schemaVersion: 1,
      recordedAt: new Date().toISOString(),
      elapsedMs,
      trustedInputs,
      interrupted,
      quality: measurementQuality({
        elapsedMs,
        activeMs: stats.activeMs,
        trustedInputs,
        interrupted,
      }),
      scenario: { nodes, edges },
      media,
      device: {
        userAgent: navigator.userAgent,
        viewport: [window.innerWidth, window.innerHeight],
        pixelRatio: window.devicePixelRatio,
        hardwareConcurrency: navigator.hardwareConcurrency,
        jsHeapBytes: memory?.usedJSHeapSize ?? null,
      },
      renderedMedia: {
        images: images.length,
        loadedImages: images.filter(
          (image) => image.complete && image.naturalWidth > 0,
        ).length,
        videos: videos.length,
        playingVideos: videos.filter((video) => !video.paused).length,
        failedMedia:
          images.filter((image) => image.complete && image.naturalWidth === 0)
            .length + videos.filter((video) => video.error !== null).length,
      },
    });
    setRunning(false);
  }

  function start() {
    setReport(null);
    const current: Recording = {
      started: performance.now(),
      inputs: inputs.current,
      windows: [],
      current: null,
      animation: 0,
      timeout: null,
    };
    recording.current = current;
    setRunning(true);
    function sample(timestamp: number) {
      if (recording.current !== current) return;
      if (isMotionSample(moving.current, lastMotion.current, timestamp)) {
        if (!current.current) {
          current.current = [];
          current.windows.push(current.current);
        }
        current.current.push(timestamp);
      } else current.current = null;
      current.animation = requestAnimationFrame(sample);
    }
    current.animation = requestAnimationFrame(sample);
    current.timeout = setTimeout(() => finish(false), 60000);
  }

  // Stop sampling on unmount; never keep background animation or timers alive.
  useEffect(
    () => () => {
      const current = recording.current;
      if (!current) return;
      cancelAnimationFrame(current.animation);
      if (current.timeout) clearTimeout(current.timeout);
      recording.current = null;
    },
    [],
  );

  useEffect(() => {
    function onVisibilityChange() {
      if (document.hidden) finish(true);
    }
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () =>
      document.removeEventListener("visibilitychange", onVisibilityChange);
  });

  function download() {
    if (!report) return;
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(report, null, 2)], { type: "application/json" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `canvas-${nodes}-${edges}-${Date.now()}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <section
      className="flex flex-wrap items-center gap-3 bg-white px-5 py-3 text-xs text-slate-700"
      aria-label="交互测量"
    >
      <Button size="sm" variant="secondary" disabled={running} onClick={start}>
        录制 60 秒交互
      </Button>
      {running ? (
        <Button size="sm" variant="outline" onClick={() => finish(false)}>
          结束冒烟录制
        </Button>
      ) : null}
      <p role="status" className="min-w-0 flex-1 leading-5 tabular-nums">
        {running
          ? "正在录制…请连续平移、缩放或拖动节点，空闲帧不计入。"
          : report
            ? `${report.quality} · 运动 ${(report.activeMs / 1000).toFixed(1)} 秒 · ${report.meanFps?.toFixed(1) ?? "—"} fps · 帧间隔 P95 ${report.p95FrameMs?.toFixed(1) ?? "—"} ms · 媒体失败 ${report.renderedMedia.failedMedia}`
            : "记录真实交互与运动帧；本机记录不代表目标设备验收。"}
      </p>
      {report ? (
        <Button size="sm" variant="outline" onClick={download}>
          导出测量 JSON
        </Button>
      ) : null}
    </section>
  );
}
