import { ArrowRight, CircleDashed } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";

export function ProductionNextAction({
  blockingReasons,
  action,
}: {
  action?: API.NextAction;
  blockingReasons: API.BlockingReason[];
}) {
  return (
    <section
      aria-label="下一步生产动作"
      className="mt-7 grid gap-5 rounded-2xl border bg-muted/35 p-5 md:grid-cols-[minmax(0,1fr)_auto] md:items-center"
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          <CircleDashed className="size-4" aria-hidden="true" />
          下一步生产动作
        </div>
        <h2 className="mt-2 text-xl font-semibold tracking-tight">
          {action?.label ?? "等待新的生产动作"}
        </h2>
        {blockingReasons.length ? (
          <div className="mt-2 grid gap-1 text-sm leading-6 text-muted-foreground">
            {blockingReasons.slice(0, 3).map((reason) => (
              <p key={`${reason.code}:${reason.resource_id}`}>{reason.summary}</p>
            ))}
          </div>
        ) : (
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            当前没有已确认的阻塞项，可以继续推进制作。
          </p>
        )}
      </div>
      {action ? (
        <Button asChild className="shrink-0">
          <Link href={action.href}>
            开始处理
            <ArrowRight aria-hidden="true" />
          </Link>
        </Button>
      ) : null}
    </section>
  );
}
