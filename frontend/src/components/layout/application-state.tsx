import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { Brand, BrandMark } from "@/components/brand/brand";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { PageLoading, type LoadingLayout } from "./loading-state";

/** Shared full-page boundary for session, route and editor startup states. */
export function ApplicationState({
  title,
  description,
  children,
}: {
  title: string;
  description: ReactNode;
  children?: ReactNode;
}) {
  return (
    <main className="application-state">
      <div className="application-state-content">
        <Brand wordmark />
        <Empty role="alert">
          <EmptyHeader>
            <EmptyMedia variant="icon" aria-hidden="true">
              <CircleAlert />
            </EmptyMedia>
            <EmptyTitle>
              <h1>{title}</h1>
            </EmptyTitle>
            <EmptyDescription>{description}</EmptyDescription>
          </EmptyHeader>
          {children ? (
            <EmptyContent className="flex-row flex-wrap justify-center gap-2">
              {children}
            </EmptyContent>
          ) : null}
        </Empty>
      </div>
    </main>
  );
}

export function ApplicationLoading({
  message = "正在加载页面…",
  layout = "form",
  sidebarOpen,
}: {
  message?: string;
  layout?: LoadingLayout;
  sidebarOpen?: boolean;
}) {
  if (sidebarOpen === undefined)
    return (
      <main className={layout === "canvas" ? undefined : "application-state"}>
        <PageLoading
          message={message}
          layout={layout}
          heading={layout !== "canvas"}
          className={layout === "canvas" ? undefined : "max-w-xl"}
        />
      </main>
    );

  return (
    <main
      className="product-shell application-loading"
      data-sidebar-open={sidebarOpen}
      data-loading-view={layout}
    >
      <div className="application-loading-sidebar" aria-hidden="true">
        <div className="application-loading-brand">
          <BrandMark />
          {sidebarOpen && <span>织光 Glowloom</span>}
        </div>
        <div className="flex flex-col gap-1">
          {[0, 1, 2, 3, 4, 5, 6].map((index) => (
            <div key={index} className="application-loading-nav-item">
              <Skeleton className="size-5 shrink-0" />
              {sidebarOpen && (
                <Skeleton
                  className={index % 2 === 0 ? "h-3 w-20" : "h-3 w-14"}
                />
              )}
            </div>
          ))}
        </div>
      </div>
      <div className="product-content">
        <div className="application-loading-header" aria-hidden="true">
          <div className="application-loading-brand application-loading-mobile-brand">
            <Skeleton className="size-6" />
            <BrandMark />
          </div>
          <Skeleton className="h-9 w-28 rounded-xl md:w-36" />
        </div>
        <div className="product-main application-loading-body">
          <PageLoading
            message={message}
            layout={layout}
            heading={layout !== "home"}
            className={layout === "home" ? undefined : "workspace-page"}
          />
        </div>
      </div>
    </main>
  );
}
