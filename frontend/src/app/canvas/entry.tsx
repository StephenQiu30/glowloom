"use client";

import dynamic from "next/dynamic";

const Editor = dynamic(
  () =>
    import("@/components/canvas/service-workspace").then(
      (module) => module.ServiceCanvasWorkspace,
    ),
  {
    ssr: false,
    loading: () => (
      <div role="status" className="flex h-dvh items-center justify-center">
        正在打开画布…
      </div>
    ),
  },
);

export function CanvasEntry() {
  return <Editor />;
}
