"use client";
import { PageHeader } from "@/components/layout/page-header";

import Link from "next/link";
import { FileQuestion } from "lucide-react";
import { usePathname } from "next/navigation";
import { resolveWorkspaceRoute } from "@/components/canvas/runtime/components/layout/lib/workspace-routes";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { ContentState } from "@/components/layout/content-state";
import { PageFailure } from "@/components/layout/application-state";
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
}: {
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <ContentState
      className="min-h-72"
      title={title}
      description={description}
      icon={FileQuestion}
      actions={
        children ?? (
          <Button asChild variant="outline">
            <Link href="/">返回首页</Link>
          </Button>
        )
      }
    />
  );
}

export function PageError({
  reset,
  title = "画布读取失败",
}: {
  reset: () => void;
  title?: string;
}) {
  const pathname = usePathname();
  const { route } = resolveWorkspaceRoute(pathname || "/");
  return (
    <section className="workspace-page page-feedback">
      <PageHeader title={route.title} />
      <PageFailure title={title} retry={reset} />
    </section>
  );
}
