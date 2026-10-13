import Link from "next/link";
import { Sparkles } from "lucide-react";
export function Brand({ wordmark = false }: { wordmark?: boolean }) {
  return (
    <Link
      href="/"
      aria-label="浮光首页"
      className="inline-flex shrink-0 items-center gap-2 text-2xl font-medium"
    >
      <Sparkles className="size-7" strokeWidth={1.4} />
      {wordmark ? "浮光" : null}
    </Link>
  );
}
