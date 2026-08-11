import { ArrowRight, FileQuestion, LogIn, ServerCrash, ShieldX } from "lucide-react";
import Link from "next/link";

import { PageHeader } from "@/components/studio/page-header";
import { StudioBrand } from "@/components/studio/studio-brand";
import { Button } from "@/components/ui/button";

type SystemStatus = "401" | "403" | "404" | "503";

const statusIcons = {
  "401": LogIn,
  "403": ShieldX,
  "404": FileQuestion,
  "503": ServerCrash,
} as const;

const statusGuidance: Record<SystemStatus, string> = {
  "401": "登录后，系统会重新读取你的成员身份与可访问入口。",
  "403": "当前身份不会加载受限内容；如需访问，请联系空间所有者。",
  "404": "请检查地址，或从项目列表重新选择要继续的内容。",
  "503": "系统不会在身份未知时展示受保护内容，请稍后重新进入。",
};

export function SystemStatusPage({
  status,
  title,
  description,
  primaryAction,
  secondaryAction,
}: {
  status: SystemStatus;
  title: string;
  description: string;
  primaryAction: { href: string; label: string };
  secondaryAction?: { href: string; label: string };
}) {
  const StatusIcon = statusIcons[status];

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header aria-label="Lanverse 全局页眉">
        <div className="mx-auto flex h-[72px] max-w-[1440px] items-center px-5 md:px-8">
          <StudioBrand size="l" />
        </div>
      </header>
      <section className="mx-auto w-full max-w-[1440px] px-5 py-12 md:px-8 md:py-14">
        <PageHeader
          actions={(
            <div aria-label="恢复访问操作" className="flex flex-wrap gap-3" role="group">
              <Button asChild className="h-10 px-4">
                <Link href={primaryAction.href}>
                  {primaryAction.label}
                  <ArrowRight aria-hidden="true" />
                </Link>
              </Button>
              {secondaryAction ? (
                <Button asChild className="h-10 px-4" variant="outline">
                  <Link href={secondaryAction.href}>{secondaryAction.label}</Link>
                </Button>
              ) : null}
            </div>
          )}
          description={description}
          eyebrow={(
            <span className="flex items-center gap-2 text-muted-foreground">
              <StatusIcon className="size-4" aria-hidden="true" />
              <span className="font-mono text-xs">HTTP {status}</span>
            </span>
          )}
          title={title}
        />

        <div className="mt-10 bg-muted/35 px-5 py-6 md:flex md:items-center md:justify-between md:gap-8 md:px-7">
          <p className="text-xs font-medium text-muted-foreground">下一步</p>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground md:mt-0 md:text-right">
            {statusGuidance[status]}
          </p>
        </div>
      </section>
    </main>
  );
}
