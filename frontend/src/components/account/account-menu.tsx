"use client";

import Link from "next/link";
import { useRef, useState, type ReactNode } from "react";
import {
  ChevronRight,
  ChevronLeft,
  LayoutGrid,
  LogOut,
  Monitor,
  Settings,
  Shield,
  Coins,
} from "lucide-react";
import { AccountAvatar } from "./account-avatar";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  ThemeMenuItems,
  useThemeMenuSelection,
} from "@/components/controls/theme-controls";
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

const itemClassName = "min-h-10 gap-2 rounded-lg px-2 max-md:min-h-11";
const accountLinks = [
  { label: "我的团队", href: "/teams" },
  { label: "邀请与授权", href: "/invitations" },
  { label: "修改密码", href: "/reset-password" },
];
const platformLinks = [
  { label: "AI 服务接入", href: "/providers" },
  { label: "模型管理", href: "/models" },
  { label: "用户管理", href: "/users" },
  { label: "平台团队", href: "/platform/teams" },
  { label: "平台设置", href: "/platform/settings" },
  { label: "通知管理", href: "/platform/notifications" },
  { label: "运行诊断", href: "/health" },
  { label: "审计日志", href: "/audit" },
];
type MenuSection = "账号设置" | "外观" | "平台管理";

function MenuLinks({ links }: { links: typeof accountLinks }) {
  return (
    <DropdownMenuGroup className="flex flex-col gap-1">
      {links.map(({ label, href }) => (
        <DropdownMenuItem key={href} asChild className={itemClassName}>
          <Link href={href}>{label}</Link>
        </DropdownMenuItem>
      ))}
    </DropdownMenuGroup>
  );
}

export function AccountMenu({
  actor,
  onLogout,
  disabled = false,
}: {
  actor: API.EmailSessionView["actor"];
  onLogout: () => void;
  disabled?: boolean;
}) {
  const isMobile = useIsMobile();
  const themeSelection = useThemeMenuSelection();
  const themeLabel =
    themeSelection.theme === "light"
      ? "浅色"
      : themeSelection.theme === "dark"
        ? "深色"
        : themeSelection.theme === "system"
          ? "跟随系统"
          : "";
  const [section, setSection] = useState<MenuSection | null>(null);
  const sectionTriggers = useRef<Partial<Record<MenuSection, HTMLElement>>>({});
  const backButton = useRef<HTMLDivElement>(null);
  function enterSection(value: MenuSection) {
    setSection(value);
    requestAnimationFrame(() => backButton.current?.focus());
  }
  function leaveSection() {
    const previous = section;
    setSection(null);
    requestAnimationFrame(() => {
      if (previous) sectionTriggers.current[previous]?.focus();
    });
  }
  function submenu(label: MenuSection, icon: ReactNode, children: ReactNode) {
    return isMobile ? (
      <DropdownMenuItem
        aria-label={label}
        ref={(node) => {
          sectionTriggers.current[label] = node ?? undefined;
        }}
        className={itemClassName}
        onSelect={(event) => {
          event.preventDefault();
          enterSection(label);
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowRight") {
            event.preventDefault();
            enterSection(label);
          }
        }}
      >
        {icon}
        <span className="flex-1">{label}</span>
        {label === "外观" ? (
          <span className="text-xs text-muted-foreground">{themeLabel}</span>
        ) : null}
        <ChevronRight aria-hidden="true" className="ml-auto" />
      </DropdownMenuItem>
    ) : (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger aria-label={label} className={itemClassName}>
          {icon}
          <span className="flex-1">{label}</span>
          {label === "外观" ? (
            <span className="text-xs text-muted-foreground">{themeLabel}</span>
          ) : null}
        </DropdownMenuSubTrigger>
        <DropdownMenuPortal>
          <DropdownMenuSubContent
            collisionPadding={16}
            sideOffset={8}
            className="max-h-(--radix-dropdown-menu-content-available-height) w-60 overflow-y-auto rounded-xl duration-150"
          >
            {children}
          </DropdownMenuSubContent>
        </DropdownMenuPortal>
      </DropdownMenuSub>
    );
  }
  const name = actor.display_name.trim() || "创作者";
  const accountName = actor.email.trim();
  const isAdmin = actor.platform_role === "platform_admin";

  return (
    <DropdownMenu onOpenChange={() => setSection(null)}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="toolbar-icon"
          aria-label="账号菜单"
          disabled={disabled}
          title={name}
        >
          <AccountAvatar />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        onKeyDown={(event) => {
          if (isMobile && section && event.key === "ArrowLeft") {
            event.preventDefault();
            leaveSection();
          }
        }}
        align="end"
        sideOffset={8}
        collisionPadding={16}
        className="w-80 max-w-[calc(100vw-2rem)] rounded-xl p-1 duration-150"
      >
        {isMobile && section ? (
          <>
            <DropdownMenuGroup>
              <DropdownMenuItem
                ref={backButton}
                aria-label="返回"
                className={itemClassName}
                onSelect={(event) => {
                  event.preventDefault();
                  leaveSection();
                }}
              >
                <ChevronLeft aria-hidden="true" />
                {section}
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            {section === "账号设置" ? <MenuLinks links={accountLinks} /> : null}
            {section === "平台管理" && isAdmin ? (
              <MenuLinks links={platformLinks} />
            ) : null}
            {section === "外观" ? (
              <div className="account-menu-theme">
                <ThemeMenuItems selection={themeSelection} />
              </div>
            ) : null}
          </>
        ) : (
          <>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="sr-only">
                当前账号
              </DropdownMenuLabel>
              <DropdownMenuItem
                asChild
                className="min-h-[72px] gap-3 rounded-lg px-3 py-3"
              >
                <Link href="/account" aria-label="个人空间">
                  <AccountAvatar className="size-9" />
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
                  </span>
                  <ChevronRight aria-hidden="true" data-icon="inline-end" />
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup className="flex flex-col gap-1">
              <DropdownMenuItem asChild className={itemClassName}>
                <Link href="/billing">
                  <Coins aria-hidden="true" />
                  积分与账单
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild className={itemClassName}>
                <Link href="/analytics">
                  <LayoutGrid aria-hidden="true" />
                  我的分析
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup className="flex flex-col gap-1">
              {submenu(
                "账号设置",
                <Settings aria-hidden="true" />,
                <MenuLinks links={accountLinks} />,
              )}
              {submenu(
                "外观",
                <Monitor aria-hidden="true" />,
                <div className="account-menu-theme">
                  <ThemeMenuItems selection={themeSelection} />
                </div>,
              )}
              {isAdmin
                ? submenu(
                    "平台管理",
                    <Shield aria-hidden="true" />,
                    <MenuLinks links={platformLinks} />,
                  )
                : null}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem
                variant="destructive"
                onSelect={onLogout}
                className={itemClassName}
              >
                <LogOut aria-hidden="true" />
                退出登录
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
