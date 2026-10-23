import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader } from "@/components/ui/empty";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { CanvasNodeType } from "@/components/canvas/types/canvas";
import {
  Image,
  AudioLines,
  FileText,
  Video,
  MousePointer2,
} from "lucide-react";

const creationTools = [
  {
    type: CanvasNodeType.Image,
    label: "图片生成",
    icon: Image,
  },
  {
    type: CanvasNodeType.Video,
    label: "视频生成",
    icon: Video,
  },
  {
    type: CanvasNodeType.Audio,
    label: "音频素材",
    icon: AudioLines,
  },
  {
    type: CanvasNodeType.Text,
    label: "脚本生成",
    icon: FileText,
  },
];

export function CanvasEmptyState({
  onCreate,
}: {
  onCreate: (type: CanvasNodeType) => void;
}) {
  return (
    <div className="canvas-start pointer-events-none absolute inset-0 flex items-center justify-center">
      <Empty className="canvas-start-content">
        <EmptyHeader>
          <EmptyDescription className="flex items-center justify-center gap-2">
            <MousePointer2 aria-hidden="true" />
            双击画布 自由生成节点
          </EmptyDescription>
        </EmptyHeader>
        <div
          data-canvas-no-zoom
          className="canvas-start-tools pointer-events-auto"
        >
          {creationTools.map(({ type, label, icon: Icon }) => (
            <Card key={label} className="canvas-launch-card">
              <Button variant="ghost" onClick={() => onCreate(type)}>
                <Icon data-icon="inline-start" aria-hidden="true" />
                <CardHeader>
                  <CardTitle>{label}</CardTitle>
                </CardHeader>
              </Button>
            </Card>
          ))}
        </div>
      </Empty>
    </div>
  );
}
