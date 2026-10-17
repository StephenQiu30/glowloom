import type { ReactNode } from "react";
import { Brand } from "@/components/brand/brand";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
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
    <main className="application-state" aria-busy="true">
      <div className="application-state-content">
        <Brand wordmark />
        <LoadingState>{message}</LoadingState>
      </div>
    </main>
  );
}
