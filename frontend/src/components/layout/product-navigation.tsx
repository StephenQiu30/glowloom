"use client";

import Link from "next/link";
import type { ComponentProps } from "react";
import { cn } from "cn";
import {
  Compass,
  Contact,
  Folder,
  LayoutDashboard,
  LogIn,
  MessageSquarePlus,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Scale,
  Sparkles,
  X,
} from "lucide-react";
import { BrandMark } from "./brand";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription } from "@/components/ui/empty";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";

const links = [
  { id: "home", label: "创作", icon: Sparkles, href: "/" },
  { id: "explore", label: "探索", icon: Compass, href: "/explore" },
  { id: "assets", label: "资产", icon: Folder, href: "/assets" },
  { id: "subjects", label: "主体", icon: Contact, href: "/subjects" },
];

export function Navigation({
  screen,
  mobile = false,
}: {
  screen: string;
  mobile?: boolean;
}) {
  const { setOpenMobile } = useSidebar();
  return (
    <SidebarGroup
      role="navigation"
      aria-label={mobile ? "移动主导航" : "主导航"}
      className={cn(mobile ? "mobile-navigation" : "primary-navigation")}
    >
      <SidebarMenu>
        {links.map(({ id, label, icon: Icon, href }) => (
          <SidebarMenuItem key={id}>
            <SidebarMenuButton
              asChild
              isActive={screen === id}
              tooltip={label}
              className="group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:p-0!"
            >
              <Link
                href={href}
                aria-label={label}
                aria-current={screen === id ? "page" : undefined}
                onClick={() => setOpenMobile(false)}
              >
                <Icon data-icon="inline-start" />
                <span>{label}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}

export function ProductNavigation({
  screen,
  onLogin,
  onMobileCloseAutoFocus,
}: {
  screen: string;
  onLogin: () => void;
  onMobileCloseAutoFocus: ComponentProps<
    typeof Sidebar
  >["onMobileCloseAutoFocus"];
}) {
  const session = useOptionalSession();
  const { open, isMobile, toggleSidebar, setOpenMobile } = useSidebar();
  const compact = !open && !isMobile;
  const toggleLabel = isMobile ? "关闭导航" : open ? "收起导航" : "展开导航";
  const ToggleIcon = isMobile ? X : open ? PanelLeftClose : PanelLeftOpen;
  const closeMobile = () => setOpenMobile(false);
  return (
    <Sidebar
      collapsible="icon"
      className="product-sidebar border-r-0"
      onMobileCloseAutoFocus={onMobileCloseAutoFocus}
    >
      <SidebarHeader
        className={cn("sidebar-brand", compact && "sidebar-brand-compact")}
      >
        <Button asChild variant="ghost" className="sidebar-home">
          <Link
            href="/"
            aria-label="织光首页"
            className="product-brand"
            onClick={closeMobile}
          >
            <BrandMark />
            {!compact && (
              <>
                <span>织光</span>
                <small>Glowloom</small>
              </>
            )}
          </Link>
        </Button>
        <Tooltip open={isMobile ? false : undefined}>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="sidebar-toggle"
              aria-label={toggleLabel}
              aria-expanded={isMobile ? true : open}
              aria-controls="product-sidebar-navigation"
              onClick={toggleSidebar}
            >
              <ToggleIcon data-icon="inline-start" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side={compact ? "right" : "bottom"}>
            {toggleLabel}
          </TooltipContent>
        </Tooltip>
      </SidebarHeader>
      <SidebarContent
        id="product-sidebar-navigation"
        className="product-sidebar-content"
      >
        <Navigation screen={screen} />
        <SidebarGroup className="sidebar-history">
          {!compact && (
            <>
              <SidebarGroupLabel>对话</SidebarGroupLabel>
              <SidebarGroupAction asChild>
                <Link href="/" aria-label="新建创作" onClick={closeMobile}>
                  <Plus />
                </Link>
              </SidebarGroupAction>
            </>
          )}
          <SidebarGroupContent>
            {compact ? (
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    tooltip="新建创作"
                    className="group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:p-0!"
                  >
                    <Link href="/" aria-label="新建创作" onClick={closeMobile}>
                      <MessageSquarePlus />
                      <span>新建创作</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            ) : (
              <Empty className="sidebar-history-empty">
                <EmptyDescription>暂无历史对话</EmptyDescription>
              </Empty>
            )}
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup className="sidebar-history">
          {!compact && (
            <>
              <SidebarGroupLabel>画布</SidebarGroupLabel>
              <SidebarGroupAction asChild>
                <Link
                  href="/projects"
                  aria-label="我的画布"
                  onClick={closeMobile}
                >
                  <Plus />
                </Link>
              </SidebarGroupAction>
            </>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  className="group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:p-0!"
                  asChild
                  tooltip="我的画布"
                  isActive={screen === "projects" || screen === "canvas"}
                >
                  <Link
                    href="/projects"
                    aria-label="查看我的画布"
                    aria-current={screen === "projects" ? "page" : undefined}
                    onClick={closeMobile}
                  >
                    <LayoutDashboard />
                    <span>查看我的画布</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="sidebar-footer">
        {!session && (
          <>
            {!compact && (
              <Empty className="sidebar-guest">
                <EmptyDescription>登录后保存你的创作</EmptyDescription>
              </Empty>
            )}
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  className="group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:p-0!"
                  tooltip="登录 / 注册"
                  aria-label="登录 / 注册"
                  onClick={() => {
                    closeMobile();
                    onLogin();
                  }}
                >
                  <LogIn />
                  <span>登录 / 注册</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </>
        )}
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              tooltip="第三方许可"
              size="sm"
              className="group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:p-0!"
            >
              <Link
                href="/licenses"
                aria-label="第三方许可"
                onClick={closeMobile}
              >
                <Scale />
                <span>第三方许可</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
