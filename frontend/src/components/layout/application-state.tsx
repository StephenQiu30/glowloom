import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { Brand } from "@/components/brand/brand";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
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
