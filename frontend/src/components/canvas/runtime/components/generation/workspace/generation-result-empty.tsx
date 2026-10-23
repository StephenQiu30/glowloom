import { Image, Music2, Type, Video } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { cn } from "@/lib/utils";

const contentKinds = {
  image: { label: "图片", icon: Image },
  video: { label: "视频", icon: Video },
  text: { label: "文本", icon: Type },
  audio: { label: "音频", icon: Music2 },
};

export function GenerationResultEmpty({
  kind,
  onRetry,
  className,
}: {
  kind: keyof typeof contentKinds;
  onRetry?: () => void;
  className?: string;
}) {
  const { label, icon: Icon } = contentKinds[kind];
  return (
    <Empty className={cn("h-full gap-3 p-4", className)}>
      <EmptyHeader className="gap-2">
        <EmptyMedia variant="icon">
          <Icon />
        </EmptyMedia>
        <EmptyTitle>暂无可展示的{label}</EmptyTitle>
      </EmptyHeader>
      {onRetry && (
        <EmptyContent>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onMouseDown={(event) => event.stopPropagation()}
            onClick={(event) => {
              event.stopPropagation();
              onRetry();
            }}
          >
            重试
          </Button>
        </EmptyContent>
      )}
    </Empty>
  );
}
