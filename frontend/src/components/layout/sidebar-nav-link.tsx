import type { ComponentProps, ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import Link from "next/link";
import { SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar";

type MenuButtonProps = ComponentProps<typeof SidebarMenuButton>;

export function SidebarNavLink({
  href,
  label,
  icon: Icon,
  active = false,
  variant,
  size,
  onNavigate,
  actions,
}: {
  href: ComponentProps<typeof Link>["href"];
  label: string;
  icon?: LucideIcon;
  active?: boolean;
  variant?: MenuButtonProps["variant"];
  size?: MenuButtonProps["size"];
  onNavigate?: () => void;
  actions?: ReactNode;
}) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        asChild
        variant={variant}
        size={size}
        isActive={active}
        tooltip={label}
        className={actions ? "pr-8" : undefined}
      >
        <Link
          href={href}
          aria-label={label}
          title={variant === "history" ? label : undefined}
          aria-current={active ? "page" : undefined}
          onClick={onNavigate}
        >
          {Icon && <Icon aria-hidden="true" />}
          <span>{label}</span>
        </Link>
      </SidebarMenuButton>
      {actions}
    </SidebarMenuItem>
  );
}
