import { Image, Music2, Type, Video } from "lucide-react";
import { Empty, EmptyHeader, EmptyMedia } from "@/components/ui/empty";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
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
      </EmptyHeader>
      <FeedbackToast
        message={label + "生成失败"}
        action={onRetry ? { label: "重试", onClick: onRetry } : undefined}
      />
    </Empty>
  );
}
