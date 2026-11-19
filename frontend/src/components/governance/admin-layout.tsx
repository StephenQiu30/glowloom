"use client";
import Link from "next/link";
import type { AdminSection } from "@/components/layout/routes";
import { useEffect, useRef, type ReactNode } from "react";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { Button } from "@/components/ui/button";

function revealNavigationLink(navigation: HTMLElement, link: Element) {
  const viewport = navigation.getBoundingClientRect();
  const item = link.getBoundingClientRect();
  const offset =
    Math.min(item.left - viewport.left, 0) ||
    Math.max(item.right - viewport.right, 0);
  if (offset) navigation.scrollLeft += offset;
}

export function AdminNavigation({ active }: { active: AdminSection }) {
  const navigationRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const navigation = navigationRef.current;
    const current = navigation?.querySelector('[aria-current="page"]');
    if (!navigation || !current) return;

    const revealCurrent = () => revealNavigationLink(navigation, current);
    revealCurrent();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(revealCurrent);
    observer.observe(navigation);
    return () => observer.disconnect();
  }, [active]);

  const links = [
    { id: "users", href: "/users", label: "用户管理" },
    { id: "teams", href: "/platform/teams", label: "团队管理" },
    { id: "providers", href: "/providers", label: "AI 服务接入" },
    { id: "models", href: "/models", label: "模型管理" },
    { id: "settings", href: "/platform/settings", label: "平台设置" },
    { id: "notifications", href: "/platform/notifications", label: "通知管理" },
    { id: "health", href: "/health", label: "运行日志" },
    { id: "audit", href: "/audit", label: "审计日志" },
  ];
  return (
    <nav
      ref={navigationRef}
      aria-label="管理页面"
      className="admin-navigation"
      onFocusCapture={(event) =>
        revealNavigationLink(event.currentTarget, event.target)
      }
    >
      {links.map((link) => (
        <Button
          key={link.id}
          variant={active === link.id ? "secondary" : "ghost"}
          asChild
          size="toolbar"
          className="admin-navigation-link focus-visible:ring-inset"
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
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <PageHeader title={title} description={description}>
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
  total?: number;
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
        已选 {count}
        {total === undefined ? "" : ` / ${total}`} 项
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
