"use client";
import type { AdminSection } from "@/components/layout/routes";
import { createContext, useContext, type ReactNode } from "react";
import { useIsMobile } from "@/hooks/use-mobile";
import { HeaderLinks } from "@/components/layout/header-navigation";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { Button } from "@/components/ui/button";

const adminLinks = [
  { id: "users", href: "/users", label: "用户管理" },
  { id: "teams", href: "/platform/teams", label: "团队管理" },
  {
    id: "providers",
    href: "/providers",
    label: "AI 服务",
    title: "AI 服务接入",
  },
  { id: "models", href: "/models", label: "模型管理" },
  { id: "settings", href: "/platform/settings", label: "平台设置" },
  { id: "notifications", href: "/platform/notifications", label: "通知管理" },
  { id: "health", href: "/health", label: "运行日志", heading: "运行诊断" },
  { id: "audit", href: "/audit", label: "审计日志" },
];
const AdminNavigationTitle = createContext<string | undefined>(undefined);

function useAdminHeadingInNavigation(title: string) {
  return title === useContext(AdminNavigationTitle);
}

export function AdminNavigation({ active }: { active: AdminSection }) {
  return <HeaderLinks active={active} label="管理页面" links={adminLinks} />;
}
/** Owned by the persistent application shell, outside route loading boundaries. */
export function AdminWorkspace({
  active,
  children,
}: {
  active: AdminSection;
  children: ReactNode;
}) {
  const link = adminLinks.find((item) => item.id === active);
  return (
    <div className="admin-workspace">
      {link && <AdminNavigation active={active} />}
      <AdminNavigationTitle.Provider
        value={link?.heading ?? link?.title ?? link?.label}
      >
        {children}
      </AdminNavigationTitle.Provider>
    </div>
  );
}
export function AdminHeading({
  title,
  description,
  children,
  detail = false,
}: {
  title: string;
  description?: ReactNode;
  children?: ReactNode;
  detail?: boolean;
}) {
  const inNavigation = useAdminHeadingInNavigation(title);
  const mobile = useIsMobile();
  return (
    <PageHeader
      title={title}
      description={description}
      hideTitle={inNavigation && !detail && !mobile}
    >
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
