"use client";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";

import Link from "next/link";
import { FileQuestion, Home, CircleAlert } from "lucide-react";
import { usePathname } from "next/navigation";
import { resolveWorkspaceRoute } from "@/components/canvas/runtime/components/layout/lib/workspace-routes";
import type { ReactNode } from "react";
import { useErrorNotification } from "./error-notification";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { PageLoading as ContentLoading } from "@/components/layout/loading-state";

export function PageLoading({ label = "正在加载" }: { label?: string }) {
  return (
    <ContentLoading
      message={label}
      layout="grid"
      heading
      className="workspace-page"
    />
  );
}

export function PageEmpty({
  title,
  description,
  children,
  heading,
  icon,
  headerAction = true,
  state = "empty",
}: {
  title: string;
  description: string;
  children?: ReactNode;
  heading?: string;
  icon?: ReactNode;
  headerAction?: boolean;
  state?: "empty" | "error";
}) {
  return (
    <section
      className={
        heading
          ? "workspace-page page-feedback"
          : "flex min-h-0 w-full min-w-0 flex-1 flex-col"
      }
    >
      {heading && (
        <PageHeader title={heading}>
          {headerAction && (
            <Button asChild variant="outline">
              <Link href="/">返回工作台</Link>
            </Button>
          )}
        </PageHeader>
      )}
      <Empty
        className="min-h-72"
        role={state === "error" ? "alert" : undefined}
      >
        <EmptyHeader>
          <EmptyMedia variant="icon" aria-hidden="true">
            {icon ?? <FileQuestion />}
          </EmptyMedia>
          <EmptyTitle>{title}</EmptyTitle>
          <EmptyDescription>{description}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent className="flex-row flex-wrap justify-center gap-2">
          {children ?? (
            <Button asChild variant="outline">
              <Link href="/">
                <Home data-icon="inline-start" />
                返回首页
              </Link>
            </Button>
          )}
        </EmptyContent>
      </Empty>
    </section>
  );
}

export function PageError({
  error,
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  const pathname = usePathname();
  const { route } = resolveWorkspaceRoute(pathname || "/");
  useErrorNotification(error, "页面加载失败");
  return (
    <PageEmpty
      heading={route.title}
      headerAction={false}
      state="error"
      icon={<CircleAlert />}
      title="暂时无法显示此页面"
      description="请重新加载内容，或返回工作台。"
    >
      <Button onClick={reset}>重新加载</Button>
      <Button asChild variant="outline">
        <Link href="/">返回工作台</Link>
      </Button>
    </PageEmpty>
  );
}
