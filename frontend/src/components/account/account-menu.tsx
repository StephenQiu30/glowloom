"use client";

import Link from "next/link";
import {
  ChevronRight,
  Folder,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Monitor,
  Shield,
} from "lucide-react";
import { AccountAvatar } from "./account-avatar";
import { useIsMobile } from "@/hooks/use-mobile";
import { ThemeMenuItems } from "@/components/controls/theme-controls";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const itemClassName = "min-h-10 gap-3 px-2 max-md:min-h-11";

export function AccountMenu({
  actor,
  onLogout,
}: {
  actor: { display_name: string };
  onLogout: () => void;
}) {
  const isMobile = useIsMobile();
  const name = actor.display_name.trim() || "创作者";
  const accountName =
    "email" in actor && typeof actor.email === "string"
      ? actor.email.trim()
      : "login_name" in actor && typeof actor.login_name === "string"
        ? actor.login_name.trim()
        : "";
  const isAdmin =
    ("platform_role" in actor && actor.platform_role === "platform_admin") ||
    ("role" in actor && actor.role === "admin");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-lg"
          className="size-10 rounded-full max-md:size-11"
          aria-label="账号菜单"
          title={name}
        >
          <AccountAvatar />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        sideOffset={8}
        collisionPadding={16}
        className="w-70 max-w-[calc(100vw-2rem)] p-2"
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel className="sr-only">当前账号</DropdownMenuLabel>
          <DropdownMenuItem asChild className="gap-3 px-2 py-3">
            <Link href="/account" aria-label="账号设置">
              <AccountAvatar size="lg" />
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate font-medium" title={name}>
                  {name}
                </span>
                <span
                  className="truncate text-xs text-muted-foreground"
                  title={accountName || undefined}
                >
                  {accountName || "账号信息暂不可用"}
                </span>
                <span className="text-xs text-muted-foreground">账号设置</span>
              </span>
              <ChevronRight data-icon="inline-end" />
            </Link>
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup className="flex flex-col gap-1">
          <DropdownMenuItem asChild className={itemClassName}>
            <Link href="/assets">
              <Folder />
              我的资产
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild className={itemClassName}>
            <Link href="/projects">
              <LayoutDashboard />
              我的画布
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild className={itemClassName}>
            <Link href="/reset-password">
              <KeyRound />
              修改密码
            </Link>
          </DropdownMenuItem>
          {isAdmin && (
            <DropdownMenuItem asChild className={itemClassName}>
              <Link href="/users">
                <Shield />
                用户管理
              </Link>
            </DropdownMenuItem>
          )}
          {isMobile ? (
            <DropdownMenuGroup className="*:data-[slot=dropdown-menu-radio-group]:*:min-h-11">
              <ThemeMenuItems />
            </DropdownMenuGroup>
          ) : (
            <DropdownMenuSub>
              <DropdownMenuSubTrigger className={itemClassName}>
                <Monitor />
                外观
              </DropdownMenuSubTrigger>
              <DropdownMenuPortal>
                <DropdownMenuSubContent collisionPadding={16}>
                  <ThemeMenuItems />
                </DropdownMenuSubContent>
              </DropdownMenuPortal>
            </DropdownMenuSub>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem onSelect={onLogout} className={itemClassName}>
            <LogOut />
            退出登录
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
