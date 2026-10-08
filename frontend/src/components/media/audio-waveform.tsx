import { cn } from "cn";

export function AudioWaveform({ playing }: { playing: boolean }) {
  return (
    <div
      className="flex h-7 items-center gap-[3px]"
      aria-label={playing ? "演示音频播放中" : "音频波形"}
    >
      {[
        4, 10, 14, 22, 17, 26, 12, 20, 25, 11, 18, 24, 15, 9, 20, 13, 6, 12,
      ].map((height, index) => (
        <span
          key={index}
          className={cn(
            "w-[2px] rounded-full bg-muted-foreground",
            playing && "animate-pulse",
          )}
          style={{ height }}
        />
      ))}
    </div>
  );
}
