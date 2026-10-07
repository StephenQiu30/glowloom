"use client";

import { Suspense, useEffect, type CSSProperties, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { cn } from "cn";
import {
  Folder,
  LayoutGrid,
  ListChecks,
  PanelLeft,
  Plus,
  Settings2,
  Waves,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import { TooltipProvider } from "@/components/ui/tooltip";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";

const navigation = [
  { href: "/projects", label: "项目", icon: LayoutGrid },
  { href: "/assets", label: "素材", icon: Folder },
  { href: "/tasks", label: "任务", icon: ListChecks },
  { href: "/settings", label: "管理", icon: Settings2 },
] as const;

export function WorkspaceShell({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider>
      <SidebarProvider
        style={
          {
            "--sidebar-width": "4.5rem",
            "--sidebar-width-icon": "4.5rem",
          } as CSSProperties
        }
      >
        <WorkspaceNavigation />
        <WorkspaceBody>{children}</WorkspaceBody>
      </SidebarProvider>
    </TooltipProvider>
  );
}

function WorkspaceBody({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { open } = useSidebar();
  const projectId = pathname.match(
    /^\/projects\/([^/]+)\/(script|bible|canvas)$/,
  )?.[1];
  const settings = pathname === "/settings";
  const links = projectId
    ? [
        { href: `/projects/${projectId}/script`, label: "剧本" },
        { href: `/projects/${projectId}/bible`, label: "设定集" },
        { href: `/projects/${projectId}/canvas`, label: "画布" },
      ]
    : settings
      ? [
          { href: "/settings?tab=project", label: "项目模型" },
          { href: "/settings?tab=defaults", label: "默认模型" },
          { href: "/settings?tab=prompts", label: "提示词偏好" },
          { href: "/settings?tab=providers", label: "供应商凭据" },
          { href: "/settings?tab=models", label: "模型注册表" },
        ]
      : [];
  return (
    <>
      {links.length > 0 && open && (
        <aside
          className="sticky top-0 hidden h-dvh w-55 shrink-0 flex-col gap-8 bg-surface-1 p-4 md:flex"
          aria-label={projectId ? "项目导航" : "管理导航"}
        >
          <Link
            href={projectId ? "/projects" : "/settings"}
            className="flex h-9 items-center gap-2 text-sm font-medium text-foreground"
          >
            {projectId ? (
              <Folder className="size-4" aria-hidden />
            ) : (
              <Settings2 className="size-4" aria-hidden />
            )}
            {projectId ? "项目工作区" : "管理"}
          </Link>
          <nav className="flex flex-col gap-1">
            {settings ? (
              <Suspense>
                <SettingsLinks links={links} />
              </Suspense>
            ) : (
              links.map(({ href, label }) => (
                <Button
                  key={href}
                  variant="navigation"
                  asChild
                  data-active={pathname === href || undefined}
                  className="justify-start"
                >
                  <Link
                    href={href}
                    aria-current={pathname === href ? "page" : undefined}
                  >
                    {label}
                  </Link>
                </Button>
              ))
            )}
          </nav>
          <p className="mt-auto text-xs text-subtle-foreground">
            浮光 · 创作工作台
          </p>
        </aside>
      )}
      <div className="min-w-0 flex-1">
        <a
          href="#workspace-main"
          className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-background focus:p-3"
        >
          跳至内容
        </a>
        <header className="flex h-14 items-center justify-between px-4 md:hidden">
          <Link href="/" className="flex items-center gap-2 font-semibold">
            <Waves className="size-5" aria-hidden />
            浮光
          </Link>
          <SidebarTrigger aria-label="打开导航" />
        </header>
        <main
          id="workspace-main"
          className={cn(
            "min-h-dvh min-w-0 p-4 md:p-6",
            (pathname === "/" || pathname === "/projects") &&
              "md:px-12 md:py-10 lg:px-16",
          )}
        >
          {children}
        </main>
      </div>
    </>
  );
}

function WorkspaceNavigation() {
  const pathname = usePathname();
  const { isMobile, setOpenMobile, toggleSidebar } = useSidebar();
  const close = () => setOpenMobile(false);
  useEffect(() => {
    const desktop = window.matchMedia("(min-width: 768px)");
    const closeOnDesktop = () => {
      if (desktop.matches) setOpenMobile(false);
    };
    desktop.addEventListener("change", closeOnDesktop);
    return () => desktop.removeEventListener("change", closeOnDesktop);
  }, [setOpenMobile]);
  return (
    <Sidebar collapsible="icon">
      <SidebarHeader className="items-center gap-4 px-3 pt-4 pb-2">
        <Button asChild size="icon" aria-label="浮光首页">
          <Link href="/" onClick={close}>
            <Waves data-icon="inline-start" />
          </Link>
        </Button>
        <Button
          asChild
          size="icon"
          variant="outline"
          className="rounded-full"
          aria-label="新建项目"
        >
          <Link href="/projects?create=true" onClick={close}>
            <Plus data-icon="inline-start" />
          </Link>
        </Button>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup className="p-3">
          <nav aria-label="工作区">
            <SidebarMenu className="gap-1">
              {navigation.map(({ href, label, icon: Icon }) => {
                const active =
                  href === "/projects"
                    ? pathname === "/" ||
                      pathname.startsWith(href) ||
                      pathname === "/canvas"
                    : pathname.startsWith(href);
                return (
                  <SidebarMenuItem key={href}>
                    <SidebarMenuButton
                      asChild
                      isActive={active}
                      size={isMobile ? "lg" : "rail"}
                      tooltip={label}
                    >
                      <Link
                        href={href}
                        onClick={close}
                        aria-current={active ? "page" : undefined}
                      >
                        <Icon aria-hidden />
                        <span>{label}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </nav>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="items-center gap-2 p-3">
        <Button
          variant="ghost"
          size="icon"
          onClick={toggleSidebar}
          aria-label="切换侧栏"
          title="切换侧栏"
        >
          <PanelLeft data-icon="inline-start" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          asChild
          aria-label="Figma 全页面预览"
          title="Figma 全页面预览"
        >
          <Link href="/preview" onClick={close}>
            <Waves data-icon="inline-start" />
          </Link>
        </Button>
        <ThemeToggle />
        <Button variant="ghost" size="icon" asChild aria-label="关于浮光与许可">
          <Link href="/licenses" onClick={close}>
            <Waves data-icon="inline-start" />
          </Link>
        </Button>
      </SidebarFooter>
    </Sidebar>
  );
}

function SettingsLinks({
  links,
}: {
  links: { href: string; label: string }[];
}) {
  const parameters = useSearchParams();
  const tab = parameters.get("tab") ?? "project";
  return links.map(({ href, label }) => {
    const active = href === `/settings?tab=${tab}`;
    return (
      <Button
        key={href}
        variant="navigation"
        asChild
        data-active={active || undefined}
        className="justify-start"
      >
        <Link href={href} aria-current={active ? "page" : undefined}>
          {label}
        </Link>
      </Button>
    );
  });
}
