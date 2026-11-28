import {
  Activity,
  Bell,
  Building2,
  ClipboardList,
  Cpu,
  Coins,
  SlidersHorizontal,
  Users,
  type LucideIcon,
} from "lucide-react";
import type { AdminSection } from "@/components/layout/routes";

export const adminDestinations: readonly {
  id: AdminSection;
  href: string;
  label: string;
  icon: LucideIcon;
}[] = [
  { id: "users", href: "/users", label: "用户管理", icon: Users },
  { id: "teams", href: "/platform/teams", label: "平台团队", icon: Building2 },
  { id: "models", href: "/models", label: "AI 服务", icon: Cpu },
  {
    id: "platform-billing",
    href: "/platform/billing",
    label: "平台账务",
    icon: Coins,
  },
  {
    id: "settings",
    href: "/platform/settings",
    label: "平台设置",
    icon: SlidersHorizontal,
  },
  {
    id: "notifications",
    href: "/platform/notifications",
    label: "通知管理",
    icon: Bell,
  },
  { id: "health", href: "/health", label: "运行诊断", icon: Activity },
  { id: "audit", href: "/audit", label: "审计日志", icon: ClipboardList },
];
