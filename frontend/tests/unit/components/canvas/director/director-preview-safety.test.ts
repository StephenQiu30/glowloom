import { expect, it } from "vitest";
import { DirectorScene } from "@/components/canvas/director/director-scene";
import type { DirectorVideoOptions } from "@/components/canvas/director/director-video-output";
it("rejects still and video output while a temporary animation preview is active", async () => {
  const engine = Object.create(DirectorScene.prototype) as DirectorScene;
  engine.setPreviewObject("actor-being-previewed");
  await expect(engine.capture()).rejects.toThrow("请先结束动作预览");
  await expect(
    engine.exportVideo(
      {} as DirectorVideoOptions,
      new AbortController().signal,
      () => {},
    ),
  ).rejects.toThrow("请先结束动作预览");
  engine.setPreviewObject(null);
  await expect(engine.capture()).rejects.toThrow("机位尚未就绪");
});
