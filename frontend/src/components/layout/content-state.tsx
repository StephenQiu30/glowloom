import type { ReactNode } from "react";
import { CircleAlert, Inbox, type LucideIcon } from "lucide-react";
import { cn } from "cn";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";

/** Shared Empty composition for a whole content region, not inline form errors. */
export function ContentState({
  title,
  description,
  icon: Icon = Inbox,
  state = "empty",
  actions,
  className,
}: {
  title: string;
  description?: ReactNode;
  icon?: LucideIcon;
  state?: "empty" | "error" | "loading";
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <Empty
      className={cn("min-h-72", className)}
      role={
        state === "error" ? "alert" : state === "loading" ? "status" : undefined
      }
      aria-busy={state === "loading" || undefined}
    >
      <EmptyHeader>
        <EmptyMedia variant="icon" aria-hidden="true">
          {state === "loading" ? (
            <Spinner role="presentation" />
          ) : state === "error" ? (
            <CircleAlert />
          ) : (
            <Icon />
          )}
        </EmptyMedia>
        <EmptyTitle>
          <h2>{title}</h2>
        </EmptyTitle>
        {description ? (
          <EmptyDescription>{description}</EmptyDescription>
        ) : null}
      </EmptyHeader>
      {actions ? (
        <EmptyContent className="flex-row flex-wrap justify-center gap-2">
          {actions}
        </EmptyContent>
      ) : null}
    </Empty>
  );
}
