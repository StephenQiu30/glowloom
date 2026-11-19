"use client";

import dynamic from "next/dynamic";
import { CanvasLoading } from "@/components/layout/loading-state";

const Editor = dynamic(
  () =>
    import("@/components/canvas/service-workspace").then(
      (module) => module.ServiceCanvasWorkspace,
    ),
  {
    ssr: false,
    loading: () => <CanvasLoading />,
  },
);

export function CanvasEntry() {
  return <Editor />;
}
