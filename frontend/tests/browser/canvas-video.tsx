/** UI-only fixture. Generate the local clip as documented in Plan 13; no upload or API writes. */
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { CanvasVideoPlayer } from "../../src/components/canvas/nodes/canvas-video-player";
import { CanvasNodeType } from "../../src/components/canvas/types/canvas";
import { Button } from "../../src/components/ui/button";
import "../../src/app/globals.css";
function Fixture() {
  const [broken, setBroken] = useState(false);
  const [pointerCount, setPointerCount] = useState(0);
  return (
    <main className="dark min-h-screen bg-background p-8 text-foreground">
      <h1>画布视频播放器隔离验收</h1>
      <p>本地合成测试片段，仅验证播放控件；不代表真实素材服务验收。</p>
      <div
        className="mt-8 h-[180px] w-[320px]"
        onPointerDown={() => setPointerCount((value) => value + 1)}
      >
        <CanvasVideoPlayer
          node={{
            id: "video-fixture",
            type: CanvasNodeType.Video,
            title: "视频播放验收",
            position: { x: 0, y: 0 },
            width: 320,
            height: 180,
            metadata: {
              content: broken
                ? "/missing-video.mp4"
                : "/tests/browser/node_modules/canvas-ui-qa/video.mp4",
            },
          }}
        />
      </div>
      <p role="status">父画布指针事件：{pointerCount}</p>
      <Button onClick={() => setBroken((value) => !value)}>
        {broken ? "恢复测试视频" : "切换失效视频"}
      </Button>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
