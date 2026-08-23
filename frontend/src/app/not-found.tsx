import { SystemStatusPage } from "@/components/system/system-status-page";

export default function NotFound() {
  return (
    <SystemStatusPage
      description="这个地址不存在、已经移动，或对应内容不再可用。你可以回到项目列表继续工作。"
      primaryAction={{ href: "/projects", label: "查看项目" }}
      secondaryAction={{ href: "/", label: "返回首页" }}
      status="404"
      title="页面不存在"
    />
  );
}
