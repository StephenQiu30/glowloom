// Gallery behavior adapted from BeefTV 1ae25027
// web/src/components/canvas/director/director-screenshot-gallery.tsx (MIT).
"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { CanvasNodeType, type CanvasNodeData } from "../model";
import { MediaPreviewDialog } from "../media-preview-dialog";
import { NodeContent } from "../nodes/node-content";
import { groupDirectorScreenshots, removeDirectorScreenshot } from "./outputs";
import type { DirectorScene } from "./model";

export function DirectorGallery({
  projectId,
  scene,
  disabled,
  onChange,
}: {
  projectId: string;
  scene: DirectorScene;
  disabled: boolean;
  onChange: (scene: DirectorScene) => void;
}) {
  const [preview, setPreview] = useState<CanvasNodeData>();
  const groups = groupDirectorScreenshots(scene);
  return (
    <section aria-label="机位截图图库" className="space-y-4">
      <p className="text-xs text-muted-foreground">
        场景截图按分镜保存，并汇总至对应机位。移除只解除图库引用。
      </p>
      {!groups.length && (
        <p className="text-sm text-muted-foreground">
          暂无截图，使用“场景截图”保存当前机位。
        </p>
      )}
      {groups.map((group) => (
        <section key={group.cameraId} className="space-y-2">
          <h3 className="text-sm font-medium">
            {group.cameraName} · {group.screenshots.length} 张
          </h3>
          <div className="grid grid-cols-2 gap-2">
            {group.screenshots.map((screenshot) => {
              const node: CanvasNodeData = {
                id: screenshot.id,
                type: CanvasNodeType.Image,
                title: screenshot.name,
                assetId: screenshot.assetId,
                position: { x: 0, y: 0 },
                width: 320,
                height: 240,
                zIndex: 0,
              };
              const cover =
                scene.cover?.assetId === screenshot.assetId &&
                scene.cover.shotId === screenshot.shotId;
              return (
                <article
                  key={screenshot.id}
                  className="min-w-0 space-y-1 rounded border p-1.5"
                >
                  <div className="aspect-video overflow-hidden rounded">
                    <NodeContent
                      node={node}
                      projectId={projectId}
                      active
                      onPreview={() => setPreview(node)}
                    />
                  </div>
                  <p className="truncate text-xs" title={screenshot.name}>
                    {screenshot.name}
                  </p>
                  <time
                    className="block text-[10px] text-muted-foreground"
                    dateTime={screenshot.createdAt}
                  >
                    {screenshot.createdAt.slice(0, 10)}
                  </time>
                  <div className="flex flex-wrap gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setPreview(node)}
                    >
                      查看
                    </Button>
                    <Button
                      size="sm"
                      variant={cover ? "secondary" : "ghost"}
                      disabled={disabled || cover}
                      onClick={() =>
                        onChange({
                          ...scene,
                          cover: {
                            assetId: screenshot.assetId,
                            shotId: screenshot.shotId,
                          },
                        })
                      }
                    >
                      {cover ? "当前封面" : "设为封面"}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={disabled}
                      aria-label={`移除截图 ${screenshot.name}`}
                      onClick={() =>
                        onChange(
                          removeDirectorScreenshot(
                            scene,
                            screenshot.shotId,
                            screenshot.id,
                          ),
                        )
                      }
                    >
                      移除
                    </Button>
                  </div>
                </article>
              );
            })}
          </div>
        </section>
      ))}
      {preview && (
        <MediaPreviewDialog
          projectId={projectId}
          node={preview}
          onClose={() => setPreview(undefined)}
        />
      )}
    </section>
  );
}
