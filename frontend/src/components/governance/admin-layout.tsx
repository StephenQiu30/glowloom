"use client";
import Link from "next/link";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
export function AdminNavigation({
  active,
}: {
  active: "users" | "teams" | "providers" | "models" | "settings";
}) {
  const links = [
    { id: "users", href: "/users", label: "用户管理" },
    { id: "teams", href: "/platform/teams", label: "团队管理" },
    { id: "providers", href: "/providers", label: "AI 服务接入" },
    { id: "models", href: "/models", label: "模型管理" },
    { id: "settings", href: "/platform/settings", label: "平台设置" },
  ];
  return (
    <nav aria-label="管理页面" className="flex flex-wrap gap-1">
      {links.map((link) => (
        <Button
          key={link.id}
          variant={active === link.id ? "secondary" : "ghost"}
          asChild
          className="max-md:h-11"
        >
          <Link
            href={link.href}
            aria-current={active === link.id ? "page" : undefined}
          >
            {link.label}
          </Link>
        </Button>
      ))}
    </nav>
  );
}
export function AdminHeading({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-medium tracking-tight">{title}</h1>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      {children ? <div className="flex flex-wrap gap-2">{children}</div> : null}
    </header>
  );
}
export function SelectionBar({
  count,
  total,
  disabled,
  onClear,
  children,
}: {
  count: number;
  total: number;
  disabled?: boolean;
  onClear: () => void;
  children: ReactNode;
}) {
  return (
    <div
      className="flex min-h-11 flex-wrap items-center gap-2"
      role="group"
      aria-label="批量操作"
    >
      {count ? (
        <>
          <span className="text-sm" aria-live="polite">
            已选 {count} / {total} 项
          </span>
          <Button variant="ghost" disabled={disabled} onClick={onClear}>
            清除选择
          </Button>
          <div className="flex flex-wrap gap-2 sm:ml-auto">{children}</div>
        </>
      ) : (
        <span className="text-sm text-muted-foreground">
          勾选记录以批量管理 · 仅当前页
        </span>
      )}
    </div>
  );
}
