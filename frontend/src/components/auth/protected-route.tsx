"use client";

import { LoaderCircle } from "lucide-react";
import { type ReactNode } from "react";

import { SystemStatusPage } from "@/components/system/system-status-page";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  canAccessPage,
  type StudioNavigation,
} from "@/lib/access-control";
import { useMeQuery } from "@/lib/server-state";

export function ProtectedRoute({
  page,
  children,
}: {
  page: Exclude<StudioNavigation, "create">;
  children: ReactNode;
}) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });

  if (sessionState === "checking" || (authenticated && me.isLoading)) {
    return (
      <div className="grid min-h-screen place-items-center">
        <LoaderCircle
          aria-label="正在核对页面权限"
          className="size-5 animate-spin text-muted-foreground"
        />
      </div>
    );
  }

  if (!authenticated) {
    return (
      <SystemStatusPage
        description="此页面属于工作空间。请登录后继续，系统会重新读取你的成员身份。"
        primaryAction={{ href: "/login", label: "前往登录" }}
        secondaryAction={{ href: "/", label: "返回首页" }}
        status="401"
        title="需要登录后继续"
      />
    );
  }

  if (me.error) {
    return (
      <SystemStatusPage
        description="暂时无法确认你的工作空间身份。系统不会在权限未知时显示受保护内容，请稍后重新进入。"
        primaryAction={{ href: "/projects", label: "返回项目" }}
        status="503"
        title="权限服务暂时不可用"
      />
    );
  }

  if (!canAccessPage(me.data?.workspace.role, page)) {
    return (
      <SystemStatusPage
        description="你当前的工作空间身份没有此页面所需权限。页面入口已从导航中隐藏，如需处理请联系空间所有者。"
        primaryAction={{ href: "/projects", label: "返回项目" }}
        status="403"
        title="无权访问此页面"
      />
    );
  }

  return children;
}
