"use client";
import Link from "next/link";
import type { AdminSection } from "@/components/layout/routes";
import type { ReactNode } from "react";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { Button } from "@/components/ui/button";
export function AdminNavigation({ active }: { active: AdminSection }) {
  const links = [
    { id: "users", href: "/users", label: "用户管理" },
    { id: "teams", href: "/platform/teams", label: "团队管理" },
    { id: "providers", href: "/providers", label: "AI 服务接入" },
    { id: "models", href: "/models", label: "模型管理" },
    { id: "settings", href: "/platform/settings", label: "平台设置" },
  ];
  return (
    <nav aria-label="管理页面" className="admin-navigation">
      {links.map((link) => (
        <Button
          key={link.id}
          variant={active === link.id ? "secondary" : "ghost"}
          asChild
          size="toolbar"
          className="admin-navigation-link"
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
/** Owned by the persistent application shell, outside route loading boundaries. */
export function AdminWorkspace({
  active,
  children,
}: {
  active: AdminSection;
  children: ReactNode;
}) {
  return (
    <div className="admin-workspace">
      <PageToolbar inHeader>
        <AdminNavigation active={active} />
      </PageToolbar>
      {children}
    </div>
  );
}
export function AdminHeading({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <PageHeader title={title}>
      {children ? (
        <ToolbarGroup
          appearance="plain"
          className="max-w-full flex-wrap"
          aria-label="页面操作"
        >
          {children}
        </ToolbarGroup>
      ) : null}
    </PageHeader>
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
  if (!count) return null;
  return (
    <ToolbarGroup
      appearance="plain"
      className="flex-wrap"
      aria-label="批量操作"
    >
      <span className="text-sm" aria-live="polite">
        已选 {count} / {total} 项
      </span>
      <Button
        size="toolbar"
        variant="ghost"
        disabled={disabled}
        onClick={onClear}
      >
        清除选择
      </Button>
      <Toolbar className="flex-wrap sm:ml-auto">{children}</Toolbar>
    </ToolbarGroup>
  );
}
