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
import { Spinner } from "@/components/ui/spinner";
import { BrandMark } from "@/components/canvas/runtime/components/layout/shell/brand-mark";
import { productBrand } from "@/components/canvas/runtime/lib/brand";

export function PageLoading({ label = "正在加载" }: { label?: string }) {
  return (
    <section
      className="flex min-h-0 w-full min-w-0 flex-1 items-center justify-center p-6 text-muted-foreground"
      aria-busy="true"
    >
      <div className="flex max-w-full flex-col items-center gap-3">
        <div className="flex items-center gap-2 text-foreground">
          <BrandMark className="size-6" />
          <span className="font-medium">{productBrand.name}</span>
        </div>
        <p className="flex items-center justify-center gap-2 text-sm">
          <Spinner
            aria-label={label}
            className="shrink-0 motion-reduce:animate-none"
          />
          <span aria-hidden="true">{label}</span>
        </p>
      </div>
    </section>
  );
}

export function PageEmpty({
  title,
  description,
  children,
  heading,
  icon,
  headerAction = true,
}: {
  title: string;
  description: string;
  children?: ReactNode;
  heading?: string;
  icon?: ReactNode;
  headerAction?: boolean;
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
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant={heading ? "default" : "icon"}>
            {icon ?? <FileQuestion />}
          </EmptyMedia>
          <EmptyTitle>{title}</EmptyTitle>
          <EmptyDescription>{description}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
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
      icon={<CircleAlert />}
      title="暂时无法显示此页面"
      description="请重新加载内容；你的账号与已保存项目不受影响。"
    >
      <Button onClick={reset}>重新加载</Button>
      <Button asChild variant="outline">
        <Link href="/">返回工作台</Link>
      </Button>
    </PageEmpty>
  );
}
