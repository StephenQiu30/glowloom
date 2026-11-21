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
import { PageLoading, type LoadingLayout } from "./loading-state";

/** Shared Empty composition for a whole content region, not inline form errors. */
export function ContentState({
  title,
  description,
  icon: Icon = Inbox,
  state = "empty",
  variant = "default",
  actions,
  className,
  loadingLayout,
}: {
  title: string;
  description?: ReactNode;
  icon?: LucideIcon | null;
  state?: "empty" | "error" | "loading";
  variant?: "default" | "workspace";
  actions?: ReactNode;
  className?: string;
  loadingLayout?: LoadingLayout;
}) {
  if (state === "loading")
    return (
      <PageLoading
        message={title}
        layout={loadingLayout}
        className={className}
      />
    );

  return (
    <Empty
      className={cn("workspace-content-state min-h-64", className)}
      role={state === "error" ? "alert" : undefined}
    >
      <EmptyHeader>
        {state === "error" ? (
          <EmptyMedia variant="icon" aria-hidden="true">
            <CircleAlert />
          </EmptyMedia>
        ) : Icon ? (
          <EmptyMedia variant="icon" aria-hidden="true">
            <Icon />
          </EmptyMedia>
        ) : null}
        <EmptyTitle variant={state === "empty" ? variant : "default"}>
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
