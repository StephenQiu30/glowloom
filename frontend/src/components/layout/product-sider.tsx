"use client";
import { NewCanvasButton } from "@/components/projects/new-canvas-button";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { RecentCanvases } from "./recent-canvases";

import Link from "next/link";
import type { ComponentProps } from "react";
import { cn } from "cn";
import {
  Compass,
  Contact,
  Folder,
  LayoutDashboard,
  MessageSquarePlus,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Sparkles,
  X,
} from "lucide-react";
import { BrandMark } from "@/components/brand/brand";
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
  { id: "assets", label: "素材库", icon: Folder, href: "/assets" },
  { id: "subjects", label: "主体库", icon: Contact, href: "/subjects" },
];

function PrimaryNavigation({ screen }: { screen: string }) {
  const { setOpenMobile } = useSidebar();
  return (
    <SidebarGroup
      role="navigation"
      aria-label="主导航"
      className="primary-navigation"
    >
      <SidebarMenu>
        {links.map(({ id, label, icon: Icon, href }) => (
          <SidebarMenuItem key={id}>
            <SidebarMenuButton
              asChild
              isActive={screen === id}
              tooltip={label}
              className="group-data-[collapsible=icon]:size-9! group-data-[collapsible=icon]:p-0!"
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

export function ProductSider({
  screen,
  onMobileCloseAutoFocus,
}: {
  screen: string;
  onMobileCloseAutoFocus: ComponentProps<
    typeof Sidebar
  >["onMobileCloseAutoFocus"];
}) {
  const { open, isMobile, toggleSidebar, setOpenMobile } = useSidebar();
  const session = useOptionalSession();
  const { selected: space, pending: scopePending } = useSpace();
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
            <BrandMark monochrome />
            {!compact && (
              <>
                <span>织光 Glowloom</span>
              </>
            )}
          </Link>
        </Button>
        <Tooltip open={isMobile ? false : undefined}>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
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
        <PrimaryNavigation screen={screen} />
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
                    className="group-data-[collapsible=icon]:size-9! group-data-[collapsible=icon]:p-0!"
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
                <EmptyDescription>
                  {session ? "对话功能待开放" : "登录后查看最近创作"}
                </EmptyDescription>
              </Empty>
            )}
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup className="sidebar-history">
          {!compact && (
            <>
              <SidebarGroupLabel asChild>
                <Link
                  href="/projects"
                  aria-label="查看我的画布"
                  onClick={closeMobile}
                >
                  画布
                </Link>
              </SidebarGroupLabel>
              <SidebarGroupAction asChild>
                <NewCanvasButton
                  variant="ghost"
                  size="icon"
                  aria-label="新建画布"
                  onClick={closeMobile}
                >
                  <Plus />
                </NewCanvasButton>
              </SidebarGroupAction>
            </>
          )}
          <SidebarGroupContent>
            {!compact && session && space && !scopePending ? (
              <RecentCanvases
                key={`${session.actor.id}/${space.id}`}
                onNavigate={closeMobile}
              />
            ) : compact ? (
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    className="group-data-[collapsible=icon]:size-9! group-data-[collapsible=icon]:p-0!"
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
                      <span>画布</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            ) : null}
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  );
}
