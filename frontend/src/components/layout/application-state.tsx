import type { ReactNode } from "react";
import { Brand } from "@/components/brand/brand";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadingState } from "./loading-state";

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
        <Empty>
          <EmptyHeader>
            <EmptyTitle>
              <h1>{title}</h1>
            </EmptyTitle>
            <EmptyDescription>{description}</EmptyDescription>
          </EmptyHeader>
          {children ? <EmptyContent>{children}</EmptyContent> : null}
        </Empty>
      </div>
    </main>
  );
}

export function ApplicationLoading({
  message = "正在加载页面…",
}: {
  message?: string;
}) {
  return (
    <ApplicationState
      title="正在准备创作空间"
      description={<LoadingState>{message}</LoadingState>}
    >
      <div className="application-loading-preview" aria-hidden="true">
        <Skeleton className="h-3 w-2/3" />
        <Skeleton className="h-3 w-1/2" />
        <Skeleton className="mt-3 h-16 w-full" />
      </div>
    </ApplicationState>
  );
}
