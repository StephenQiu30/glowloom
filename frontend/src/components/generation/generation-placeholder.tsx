"use client";
import { Progress } from "@ui-sdk/react/progress";
import { Skeleton } from "@/components/ui/skeleton";
export function GenerationPlaceholder() {
  return (
    <Progress value={null} aria-label="生成中" className="size-full">
      <Skeleton
        variant="canvas-generation"
        className="canvas-generation-shimmer size-full animate-none overflow-hidden rounded-[inherit]"
        aria-hidden
      >
        <svg
          viewBox="0 0 320 320"
          preserveAspectRatio="xMidYMid slice"
          className="size-full"
          aria-hidden
        >
          {Array.from({ length: 144 }, (_, i) => (
            <path
              key={i}
              d={`M${6 + (i % 12) * 28} ${4 + Math.floor(i / 12) * 28}q0 2 2 2q-2 0-2 2q0-2-2-2q2 0 2-2`}
            />
          ))}
        </svg>
      </Skeleton>
    </Progress>
  );
}
