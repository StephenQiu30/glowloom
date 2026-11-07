"use client";
import { NewCanvasButton } from "@/components/projects/new-canvas-button";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import {
  ConversationHistory,
  NewConversationLink,
} from "@/components/conversations/conversation-history";
import { RecentCanvases } from "./recent-canvases";
import { SidebarNavLink } from "./sidebar-nav-link";
import type { Screen } from "./routes";

import Link from "next/link";
import { useId, useState, type ComponentProps, type ReactNode } from "react";
import { cn } from "cn";
import {
  ChevronDown,
  Clapperboard,
  Compass,
  ContactRound,
  FolderClosed,
  Grid2x2Plus,
  Plus,
  Sparkles,
  SquarePen,
  X,
} from "lucide-react";
import { BrandMark } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";

const links = [
  { id: "home", label: "创作", icon: Sparkles, href: "/" },
  { id: "explore", label: "探索", icon: Compass, href: "/explore" },
  { id: "ai-tools", label: "AI工具", icon: Grid2x2Plus, href: "/ai-tools" },
  { id: "assets", label: "资产", icon: FolderClosed, href: "/assets" },
  { id: "subjects", label: "主体", icon: ContactRound, href: "/subjects" },
] as const;

function PrimaryNavigation({ screen }: { screen: Screen }) {
  const { setOpenMobile } = useSidebar();
  return (
    <SidebarGroup
      role="navigation"
      aria-label="主导航"
      className="primary-navigation"
    >
      <SidebarMenu>
        {links.map(({ id, label, icon, href }) => (
          <SidebarNavLink
            key={id}
            href={href}
            label={label}
            icon={icon}
            active={screen === id}
            variant="workspace"
            size="workspace"
            onNavigate={() => setOpenMobile(false)}
          />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}

function NavigationSection({
  title,
  open,
  onOpenChange,
  actions,
  children,
}: {
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  actions: ReactNode;
  children: ReactNode;
}) {
  const contentId = useId();
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} asChild>
      <SidebarGroup className="sidebar-section">
        <div className="sidebar-section-header">
          <CollapsibleTrigger asChild aria-controls={contentId}>
            <Button variant="navigation" className="sidebar-section-toggle">
              {title}
              <ChevronDown aria-hidden="true" data-icon="inline-end" />
            </Button>
          </CollapsibleTrigger>
          <div className="sidebar-section-actions">{actions}</div>
        </div>
        <CollapsibleContent
          id={contentId}
          forceMount
          aria-hidden={!open}
          className="sidebar-section-content"
        >
          <SidebarGroupContent>{children}</SidebarGroupContent>
        </CollapsibleContent>
      </SidebarGroup>
    </Collapsible>
  );
}

export function ProductSider({
  screen,
  onMobileCloseAutoFocus,
}: {
  screen: Screen;
  onMobileCloseAutoFocus: ComponentProps<
    typeof Sidebar
  >["onMobileCloseAutoFocus"];
}) {
  const { open, isMobile, setOpenMobile } = useSidebar();
  const [conversationsOpen, setConversationsOpen] = useState(true);
  const [canvasesOpen, setCanvasesOpen] = useState(true);
  const session = useOptionalSession();
  const { selected: space, pending: scopePending } = useSpace();
  const compact = !open && !isMobile;
  const toggleLabel = isMobile ? "关闭导航" : open ? "收起导航" : "展开导航";
  const closeMobile = () => setOpenMobile(false);
  return (
    <Sidebar
      role="complementary"
      aria-label="侧栏导航"
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
            {!compact && <span>织光 Glowloom</span>}
          </Link>
        </Button>
        {isMobile ? (
          <Button
            variant="ghost"
            size="icon-lg"
            className="sidebar-toggle"
            aria-label={toggleLabel}
            onClick={closeMobile}
          >
            <X aria-hidden="true" />
          </Button>
        ) : (
          <Tooltip>
            <TooltipTrigger asChild>
              <SidebarTrigger
                className="sidebar-toggle"
                aria-label={toggleLabel}
                aria-expanded={open}
                aria-controls="product-sidebar-navigation"
              />
            </TooltipTrigger>
            <TooltipContent side={compact ? "right" : "bottom"}>
              {toggleLabel}
            </TooltipContent>
          </Tooltip>
        )}
      </SidebarHeader>
      <SidebarContent
        id="product-sidebar-navigation"
        className="product-sidebar-content"
      >
        <PrimaryNavigation screen={screen === "projects" ? "assets" : screen} />
        <SidebarSeparator className={compact ? "mx-3 my-1" : "mx-5 my-1"} />
        <SidebarGroup
          role="navigation"
          aria-label="工作室"
          className="primary-navigation"
        >
          <SidebarMenu>
            <SidebarNavLink
              href="/short-drama"
              label="短剧工作室"
              icon={Clapperboard}
              active={screen === "short-drama"}
              variant="workspace"
              size="workspace"
              onNavigate={closeMobile}
            />
          </SidebarMenu>
        </SidebarGroup>
        {compact ? (
          <SidebarGroup className="sidebar-compact-actions">
            <SidebarMenu>
              <SidebarNavLink
                href="/"
                label="新建创作"
                icon={SquarePen}
                size="workspace"
                onNavigate={closeMobile}
              />
              <SidebarMenuItem>
                <SidebarMenuButton asChild size="workspace" tooltip="新建画布">
                  <NewCanvasButton
                    variant="ghost"
                    size="icon"
                    aria-label="新建画布"
                    onClick={closeMobile}
                  >
                    <Plus aria-hidden="true" />
                  </NewCanvasButton>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        ) : (
          <>
            <NavigationSection
              title="对话"
              open={conversationsOpen}
              onOpenChange={setConversationsOpen}
              actions={
                <NewConversationLink
                  className="sidebar-section-create"
                  onNavigate={closeMobile}
                >
                  <SquarePen aria-hidden="true" />
                </NewConversationLink>
              }
            >
              <ConversationHistory onNavigate={closeMobile} />
            </NavigationSection>
            <NavigationSection
              title="画布"
              open={canvasesOpen}
              onOpenChange={setCanvasesOpen}
              actions={
                <>
                  <Button
                    asChild
                    variant="navigation"
                    size="xs"
                    className="sidebar-section-all"
                  >
                    <Link
                      href="/assets?view=canvas"
                      aria-label="查看我的画布"
                      onClick={closeMobile}
                    >
                      全部
                    </Link>
                  </Button>
                  <NewCanvasButton
                    variant="navigation"
                    size="icon-xs"
                    className="sidebar-section-create"
                    aria-label="新建画布"
                    onClick={closeMobile}
                  >
                    <Plus aria-hidden="true" />
                  </NewCanvasButton>
                </>
              }
            >
              {session && space && !scopePending ? (
                <RecentCanvases
                  key={`${session.actor.id}/${space.id}`}
                  onNavigate={closeMobile}
                />
              ) : null}
            </NavigationSection>
          </>
        )}
      </SidebarContent>
    </Sidebar>
  );
}
