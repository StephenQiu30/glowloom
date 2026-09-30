import { Suspense } from "react";
import { CanvasWorkspace } from "@/features/canvas/workspace";

export const metadata = { title: "无限画布 · Lanverse" };
export default function CanvasPage() {
  return (
    <Suspense
      fallback={
        <main className="p-10">
          <p role="status">正在加载画布工作区…</p>
        </main>
      }
    >
      <CanvasWorkspace />
    </Suspense>
  );
}
