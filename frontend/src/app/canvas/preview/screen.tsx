"use client";

import { useLayoutEffect, useState, useRef } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { SessionContext } from "@/components/auth/session";
import { SpaceContext } from "@/components/spaces/space-context";
import { setCreationScope } from "@/lib/request";
import { installDevelopmentTransport } from "@/lib/development-transport";
import {
  CanvasPreviewData,
  previewSession,
  previewSpace,
  previewScope,
} from "./data";

const CanvasEditor = dynamic(() =>
  import("../editor").then((module) => module.CanvasEditor),
);
export function CanvasPreview() {
  const [data, setData] = useState<CanvasPreviewData | null>(null);
  const [error, setError] = useState("");
  const [epoch, setEpoch] = useState(0);
  const [session] = useState(previewSession);
  const retained = useRef<CanvasPreviewData | null>(null);
  useLayoutEffect(() => {
    if (process.env.NODE_ENV !== "development") return;
    const source = retained.current ?? CanvasPreviewData.load();
    retained.current = source;
    source.activate();
    const remove = installDevelopmentTransport({
      matches: (path) => path === "/canvas/preview",
      request: (url, options) => source.request(url, options),
    });
    setCreationScope(previewScope);
    let active = true;
    queueMicrotask(() => {
      if (active) setData(source);
    });
    return () => {
      active = false;
      remove();
      source.dispose();
      setCreationScope(null);
    };
  }, []);
  async function reset(count: number) {
    try {
      await data?.reset(count);
      setEpoch((n) => n + 1);
    } catch {
      setError("无法重置演示数据，请重新打开预览。");
    }
  }
  if (!data)
    return (
      <p role="status" className="p-6">
        {error || "正在加载画布 Mock 数据…"}
      </p>
    );
  return (
    <SessionContext value={session}>
      <SpaceContext
        value={{
          selected: previewSpace,
          spaces: [previewSpace],
          pending: false,
          error: "",
          refresh: async () => {},
          select: async () => {},
        }}
      >
        <section className="canvas-preview" aria-label="画布Mock预览">
          <div
            className="flex min-h-11 flex-wrap items-center gap-2 border-b border-border bg-muted px-3 py-1 text-xs"
            role="region"
            aria-label="Mock预览状态"
          >
            <strong>Mock 数据</strong>
            <span>仅当前页面内存 · 刷新重置 · 不写入真实项目</span>
            <Button size="xs" variant="outline" onClick={() => void reset(3)}>
              重置示例
            </Button>
            <Button size="xs" variant="ghost" onClick={() => void reset(0)}>
              空白画布
            </Button>
            <Button size="xs" variant="ghost" onClick={() => void reset(200)}>
              200 节点
            </Button>
            <Button size="xs" variant="ghost" onClick={() => void reset(2000)}>
              2000 节点
            </Button>
            <Button size="xs" variant="ghost" asChild>
              <Link href="/">退出预览</Link>
            </Button>
            {error && <span role="alert">{error}</span>}
          </div>
          <div className="min-h-0 flex-1">
            <CanvasEditor
              key={epoch}
              developmentPreview
              project={data.project}
              initial={structuredClone(data.state.canvas)}
            />
          </div>
        </section>
      </SpaceContext>
    </SessionContext>
  );
}
