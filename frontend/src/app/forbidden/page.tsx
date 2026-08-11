import type { Metadata } from "next";

import { SystemStatusPage } from "@/components/system/system-status-page";

export const metadata: Metadata = {
  title: "无权访问 · Lanverse",
};

export default function ForbiddenPage() {
  return (
    <SystemStatusPage
      description="你当前的工作空间身份没有此页面所需权限。请返回可访问区域，或联系空间所有者调整身份。"
      primaryAction={{ href: "/", label: "返回首页" }}
      secondaryAction={{ href: "/projects", label: "查看项目" }}
      status="403"
      title="无权访问此页面"
    />
  );
}
