import { type ReactNode } from "react";
import { cn } from "cn";
import { Activity, ImageIcon, Play, UserRound } from "lucide-react";

export function Placeholder({
  className,
  label,
  kind = "image",
  showIcon = true,
  labelStyle = "caption",
  surface = "raised",
  children,
}: {
  className?: string;
  label?: string;
  kind?: "image" | "video" | "audio" | "portrait";
  showIcon?: boolean;
  labelStyle?: "caption" | "badge";
  surface?: "card" | "raised";
  children?: ReactNode;
}) {
  const Icon =
    kind === "video"
      ? Play
      : kind === "audio"
        ? Activity
        : kind === "portrait"
          ? UserRound
          : ImageIcon;
  return (
    <div
      className={cn(
        "relative flex items-center justify-center overflow-hidden rounded-lg text-subtle-foreground",
        surface === "card" ? "bg-surface-2" : "bg-surface-3",
        className,
      )}
    >
      {showIcon ? (
        <Icon
          className="size-5 opacity-40"
          fill={kind === "video" ? "currentColor" : "none"}
          aria-hidden="true"
        />
      ) : null}
      {label ? (
        <span
          className={cn(
            "absolute text-[11px] text-muted-foreground",
            labelStyle === "badge"
              ? "bottom-3 left-3 rounded-sm bg-background px-1.5 py-0.5 font-mono"
              : "bottom-3 left-3",
          )}
        >
          {label}
        </span>
      ) : null}
      {children}
    </div>
  );
}
