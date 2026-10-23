"use client";

import dynamic from "next/dynamic";
import { ContentState } from "@/components/layout/content-state";

const Editor = dynamic(
  () =>
    import("@/components/canvas/service-workspace").then(
      (module) => module.ServiceCanvasWorkspace,
    ),
  {
    ssr: false,
    loading: () => (
      <ContentState
        className="h-dvh"
        state="loading"
        title="正在打开画布"
        description="正在连接你的创作空间。"
      />
    ),
  },
);

export function CanvasEntry() {
  return <Editor />;
}
