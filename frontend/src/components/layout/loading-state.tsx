import type { ReactNode } from "react";
import { cn } from "cn";
import { Spinner } from "@/components/ui/spinner";
import { Skeleton } from "@/components/ui/skeleton";
import type { Screen } from "./routes";

/** Announce the real pending operation once; the spinner is decorative. */
export function LoadingState({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      role="status"
      className={cn(
        "flex min-w-0 items-center gap-2 text-sm text-muted-foreground",
        className,
      )}
    >
      <Spinner role="presentation" aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
}

export type LoadingLayout =
  "home" | "grid" | "table" | "list" | "form" | "conversation" | "canvas";

export function loadingLayoutForScreen(screen: Screen): LoadingLayout {
  switch (screen) {
    case "home":
    case "login":
    case "register":
      return "home";
    case "assets":
    case "subjects":
    case "projects":
    case "explore":
    case "ai-tools":
    case "motion-studio":
    case "marketing-studio":
    case "billing":
      return "grid";
    case "conversations":
    case "short-drama":
      return "conversation";
    case "canvas":
      return "canvas";
    case "platform/settings":
    case "account":
    case "reset-password":
      return "form";
    case "users":
    case "models":
    case "providers":
    case "audit":
    case "health":
    case "analytics":
    case "platform/teams":
    case "platform/notifications":
    case "teams":
      return "table";
    default:
      return "list";
  }
}

const placeholderRows = [0, 1, 2, 3] as const;

function MediaGrid() {
  return (
    <div className="resource-card-grid">
      {placeholderRows.map((index) => (
        <div key={index} className="flex min-w-0 flex-col gap-2">
          <Skeleton className="aspect-video w-full rounded-xl" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      ))}
    </div>
  );
}

export function ConversationMessageSkeleton({
  className,
}: {
  className?: string;
}) {
  return (
    <div
      aria-hidden="true"
      className={cn("flex flex-col items-start gap-4", className)}
    >
      <Skeleton className="h-9 w-2/5 self-end rounded-2xl" />
      <Skeleton className="h-4 w-4/5" />
      <Skeleton className="h-4 w-3/5" />
    </div>
  );
}

function LoadingBody({ layout }: { layout: LoadingLayout }) {
  switch (layout) {
    case "home":
      return (
        <div className="home-page">
          <div className="creation-hero flex flex-col items-center gap-4 md:gap-6">
            <Skeleton className="h-9 w-full max-w-sm md:h-12 md:max-w-md" />
            <Skeleton className="h-10 w-56 rounded-full max-md:h-12 max-md:w-full" />
            <div className="flex h-40 w-full flex-col justify-between rounded-3xl border bg-card p-4 max-md:h-54">
              <div className="flex items-start gap-3">
                <Skeleton className="h-12 w-10 shrink-0 rounded-lg max-md:h-16 max-md:w-12" />
                <Skeleton className="mt-1 h-4 w-3/5" />
              </div>
              <div className="flex items-center gap-2">
                <Skeleton className="h-8 w-24 rounded-full" />
                <Skeleton className="h-8 w-16 rounded-full" />
                <Skeleton className="ml-auto size-9 shrink-0 rounded-full" />
              </div>
            </div>
          </div>
          <div className="home-destinations flex flex-col gap-3">
            <Skeleton className="h-5 w-24" />
            <div className="home-destination-grid">
              <Skeleton className="home-feature rounded-xl" />
              <div className="home-destination-links">
                {placeholderRows.map((index) => (
                  <Skeleton
                    key={index}
                    className="h-[78px] rounded-xl max-md:h-[58px]"
                  />
                ))}
              </div>
            </div>
          </div>
          <div className="inspiration flex flex-col gap-4">
            <Skeleton className="h-5 w-24" />
            <MediaGrid />
          </div>
        </div>
      );
    case "grid":
      return <MediaGrid />;
    case "table":
      return (
        <div className="flex flex-col divide-y rounded-xl border border-border">
          <div className="grid grid-cols-3 gap-4 bg-muted/40 px-4 py-4 md:grid-cols-4">
            {placeholderRows.map((index) => (
              <Skeleton key={index} className="h-3 w-2/3 last:max-md:hidden" />
            ))}
          </div>
          {placeholderRows.map((index) => (
            <div
              key={index}
              className="grid grid-cols-3 gap-4 px-4 py-4 md:grid-cols-4"
            >
              <Skeleton className="h-4 w-4/5" />
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-4 w-1/2" />
              <Skeleton className="hidden h-4 w-1/2 md:block" />
            </div>
          ))}
        </div>
      );
    case "form":
      return (
        <div className="flex w-full max-w-xl flex-col gap-6">
          {placeholderRows.slice(0, 3).map((index) => (
            <div key={index} className="flex flex-col gap-2">
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-10 w-full max-md:h-11" />
            </div>
          ))}
        </div>
      );
    case "conversation":
      return (
        <div className="mx-auto flex min-h-[min(60dvh,32rem)] w-full max-w-3xl flex-col justify-between gap-10">
          <ConversationMessageSkeleton />
          <Skeleton className="h-36 w-full rounded-3xl" />
        </div>
      );
    case "canvas":
      return (
        <div className="flex min-h-dvh w-full flex-col justify-between gap-4 p-3 md:p-4">
          <div className="flex items-center justify-between gap-3">
            <Skeleton className="h-10 w-32 md:w-52" />
            <Skeleton className="h-10 w-22 md:w-36" />
          </div>
          <Skeleton className="hidden h-56 w-11 rounded-xl md:block" />
          <div className="flex justify-between gap-3">
            <Skeleton className="h-10 w-28 md:w-44" />
            <Skeleton className="h-10 w-24 md:w-32" />
          </div>
        </div>
      );
    case "list":
      return (
        <div className="flex flex-col divide-y">
          {placeholderRows.slice(0, 3).map((index) => (
            <div
              key={index}
              className="flex min-w-0 items-center gap-3 px-2 py-3.5"
            >
              <Skeleton className="size-9 shrink-0" />
              <div className="flex min-w-0 flex-1 flex-col gap-2">
                <Skeleton className="h-4 w-3/5" />
                <Skeleton className="h-3 w-4/5" />
              </div>
            </div>
          ))}
        </div>
      );
  }
}

/** The visual placeholder is decorative. Announce the pending read once. */
export function PageLoading({
  message = "正在加载…",
  layout = "list",
  heading = false,
  className,
}: {
  message?: string;
  layout?: LoadingLayout;
  heading?: boolean;
  className?: string;
}) {
  return (
    <div
      data-loading-layout={layout}
      className={cn("w-full min-w-0", className)}
    >
      <span role="status" className="sr-only">
        {message}
      </span>
      <div aria-busy="true" aria-hidden="true" className="flex flex-col gap-6">
        {heading && <Skeleton className="h-8 w-40 max-w-full md:h-9" />}
        <LoadingBody layout={layout} />
      </div>
    </div>
  );
}

export function CanvasLoading({
  message = "正在打开画布…",
}: {
  message?: string;
}) {
  return <PageLoading layout="canvas" message={message} />;
}
