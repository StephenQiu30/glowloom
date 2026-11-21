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
  icon,
  state = "empty",
  variant = "default",
  heading = "h2",
  actions,
  className,
  loadingLayout,
}: {
  title: string;
  description?: ReactNode;
  icon?: LucideIcon | null;
  state?: "empty" | "error" | "loading";
  variant?: "default" | "workspace";
  heading?: "h1" | "h2";
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

  const Icon =
    icon === undefined ? (state === "error" ? CircleAlert : Inbox) : icon;
  const Heading = heading;
  return (
    <Empty
      className={cn("workspace-content-state min-h-64", className)}
      role={state === "error" ? "alert" : undefined}
    >
      <EmptyHeader>
        {Icon ? (
          <EmptyMedia variant="icon" aria-hidden="true">
            <Icon />
          </EmptyMedia>
        ) : null}
        <EmptyTitle variant={state === "empty" ? variant : "default"}>
          <Heading>{title}</Heading>
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
