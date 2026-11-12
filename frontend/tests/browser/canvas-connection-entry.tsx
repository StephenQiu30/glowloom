/** UI-only fixture. Admission is controlled locally; no save, upload or generation. */
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "../../src/components/ui/tooltip";
import { Button } from "../../src/components/ui/button";
import { Toaster } from "../../src/components/ui/sonner";
import { CanvasProviders } from "../../src/components/canvas/editor";
import CanvasPage from "../../src/components/canvas/editor/canvas-editor";
import type { CanvasEditorState } from "../../src/components/canvas/lib/canvas-document-adapter";
import "../../src/app/globals.css";

const initialState: CanvasEditorState = {
  nodes: [
    {
      id: "connection-source",
      type: "text",
      title: "参考文本",
      position: { x: 260, y: 160 },
      width: 280,
      height: 280,
      metadata: { content: "画布连线交互验证" },
    },
    {
      id: "connection-target",
      type: "video",
      title: "视频素材",
      position: { x: 740, y: 200 },
      width: 320,
      height: 180,
      metadata: {},
    },
  ],
  connections: [],
  backgroundMode: "dots",
  showImageInfo: false,
  viewport: { x: 0, y: 0, k: 1 },
};

function Fixture() {
  const [ready, setReady] = useState(true);
  return (
    <ThemeProvider attribute="class" forcedTheme="dark">
      <TooltipProvider>
        <CanvasProviders>
          <Toaster />
          <div className="flex h-dvh">
            <CanvasPage
              documentIntegration={{
                projectId: "22222222-2222-4222-8222-222222222222",
                initialState,
                viewStateKey: "canvas-connection-entry-fixture",
                mediaEnabled: false,
                canNavigate: true,
                frozen: false,
                admitConnectionGraph: () => ready,
                onChange: () => {},
                toolbar: () => null,
              }}
            />
          </div>
          <Button
            className="fixed right-4 bottom-16"
            variant="secondary"
            onClick={() => setReady((value) => !value)}
          >
            {ready ? "模拟保存待确认" : "恢复图接受"}
          </Button>
        </CanvasProviders>
      </TooltipProvider>
    </ThemeProvider>
  );
}

createRoot(document.getElementById("root")!).render(<Fixture />);
