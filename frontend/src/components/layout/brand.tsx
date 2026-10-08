import Link from "next/link";
import { cn } from "cn";
import { Waves } from "lucide-react";
import { screenHref } from "@/components/layout/routes";

export function Brand({ wordmark = false }: { wordmark?: boolean }) {
  return (
    <Link
      href={screenHref("home")}
      aria-label="浮光首页"
      className="inline-flex items-center gap-2.5 text-sm font-medium"
    >
      <span
        className={cn(
          "flex items-center justify-center rounded-lg bg-primary text-primary-foreground",
          wordmark ? "size-7" : "size-9",
        )}
      >
        <Waves className="size-4" />
      </span>
      {wordmark ? "浮光" : null}
    </Link>
  );
}
