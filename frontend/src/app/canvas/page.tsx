import { Suspense } from "react";
import { LiveWorkspace } from "@/features/canvas/live-workspace";

export const metadata = { title: "真实备注画布 · Lanverse" };
export default function CanvasPage() {
  return (
    <Suspense
      fallback={
        <main className="p-10">
          <p role="status">正在加载画布工作区…</p>
        </main>
      }
    >
      <LiveWorkspace />
    </Suspense>
  );
}
