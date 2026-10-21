"use client";

import { useLayoutEffect, useState, useRef } from "react";
import dynamic from "next/dynamic";
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
  if (!data)
    return (
      <p role="status" className="p-6">
        正在加载画布…
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
        <section className="canvas-preview" aria-label="画布">
          <div className="min-h-0 flex-1">
            <CanvasEditor
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
