"use client";

import {
  Bell,
  Check,
  ChevronDown,
  Command as CommandIcon,
  Folder,
  Home,
  LogOut,
  Search,
  Settings,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { StudioBrand } from "@/components/studio/studio-brand";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
} from "@/components/ui/navigation-menu";
import { Separator } from "@/components/ui/separator";
import { canAccessPage, type StudioNavigation, visiblePrimaryNavigation, type WorkspaceRole } from "@/lib/access-control";
import { clearAccessToken } from "@/lib/auth-session";
import { cn } from "@/lib/class-names";
import { useLogoutMutation } from "@/lib/server-state";

import { LayoutContainer } from "./layout-container";
import { ThemeToggle } from "./theme-toggle";

export type LayoutAuthState = "loading" | "anonymous" | "authenticated";

export type LayoutViewer = {
  displayName: string;
  workspaceName: string;
};

const productionStages = ["剧本解析", "资产", "分镜", "生成", "审核", "交付"];

const navigationItems: Array<{
  id: StudioNavigation;
  label: string;
  description: string;
  href: string;
  icon: typeof Home;
}> = [
  { id: "create", label: "首页", description: "欢迎与工作概览", href: "/", icon: Home },
  { id: "projects", label: "项目", description: "项目与短剧生产", href: "/projects", icon: Folder },
  { id: "settings", label: "空间", description: "账户与工作空间", href: "/workspaces", icon: Settings },
];

const roleLabels: Record<WorkspaceRole, string> = {
  owner: "所有者",
  editor: "编辑者",
  viewer: "查看者",
};

function ProductionProgress({ currentStep }: { currentStep: number }) {
  return (
    <div className="bg-background" aria-label="制作进度">
      <LayoutContainer className="flex min-h-12 items-center gap-4 overflow-x-auto py-1">
        <span className="shrink-0 text-xs font-medium text-muted-foreground">制作阶段</span>
        <Separator className="h-4" orientation="vertical" />
        <ol className="flex min-w-max flex-1 items-center gap-1">
          {productionStages.map((stage, index) => (
            <li className="flex items-center" key={stage}>
              <span
                className={cn(
                  "inline-flex h-7 items-center gap-1.5 px-2 text-xs",
                  index === currentStep ? "font-medium text-foreground" : "text-muted-foreground",
                )}
                aria-current={index === currentStep ? "step" : undefined}
              >
                {index < currentStep ? <Check className="size-3.5" aria-hidden="true" /> : <span className="font-mono">0{index + 1}</span>}
                {stage}
              </span>
              {index < productionStages.length - 1 ? <span className="mx-1 h-px w-5 bg-border lg:w-9" /> : null}
            </li>
          ))}
        </ol>
      </LayoutContainer>
    </div>
  );
}

function StudioNavigationMenu({
  active,
  mobile = false,
  role,
}: {
  active?: StudioNavigation;
  mobile?: boolean;
  role?: WorkspaceRole;
}) {
  const visibleIds = visiblePrimaryNavigation(role);
  const visibleItems = navigationItems.filter((item) => visibleIds.includes(item.id));

  return (
    <NavigationMenu
      aria-label={mobile ? "移动端导航" : "主导航"}
      className={cn(mobile && "w-full max-w-none justify-start")}
      viewport={false}
    >
      <NavigationMenuList className={cn("gap-1", mobile && "justify-start")}>
        {visibleItems.map((item) => (
          <NavigationMenuItem key={item.id}>
            <NavigationMenuLink
              active={active === item.id}
              asChild
              className={cn(
                "h-8 whitespace-nowrap px-2.5 py-1.5",
                active === item.id && "bg-muted/70 font-medium",
              )}
            >
              <Link href={item.href}>
                <item.icon className="size-3.5" aria-hidden="true" />
                {item.label}
              </Link>
            </NavigationMenuLink>
          </NavigationMenuItem>
        ))}
      </NavigationMenuList>
    </NavigationMenu>
  );
}

function NavigationPlaceholder({ mobile = false }: { mobile?: boolean }) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "bg-muted/35",
        mobile ? "h-8 w-56" : "h-8 w-56",
      )}
    />
  );
}

function GlobalSearch({ role }: { role: WorkspaceRole }) {
  const [open, setOpen] = useState(false);
  const visibleItems = navigationItems.filter(
    (item) => item.id !== "create" && canAccessPage(role, item.id),
  );

  useEffect(() => {
    function handleShortcut(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    }

    document.addEventListener("keydown", handleShortcut);
    return () => document.removeEventListener("keydown", handleShortcut);
  }, []);

  return (
    <CommandDialog
      description="搜索页面，或直接选择一个工作区入口。"
      onOpenChange={setOpen}
      open={open}
      title="前往 Lanverse"
      trigger={(
        <Button
          aria-label="搜索或执行命令"
          className="hidden w-64 justify-start text-muted-foreground xl:flex"
          variant="outline"
        >
          <Search aria-hidden="true" />
          <span>搜索或执行命令…</span>
          <kbd className="ml-auto inline-flex items-center gap-0.5 text-[11px] text-muted-foreground">
            <CommandIcon aria-hidden="true" className="size-3" />
            <span className="font-mono">K</span>
          </kbd>
        </Button>
      )}
    >
      <Command label="全局搜索">
        <CommandInput aria-label="全局搜索" placeholder="搜索页面或命令…" />
        <CommandList>
          <CommandEmpty>没有匹配的页面或命令。</CommandEmpty>
          <CommandGroup heading="快速入口">
            {visibleItems.map((item) => (
              <CommandDestination item={item} key={item.id} onNavigate={() => setOpen(false)} />
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </CommandDialog>
  );
}

function CommandDestination({
  item,
  onNavigate,
}: {
  item: (typeof navigationItems)[number];
  onNavigate: () => void;
}) {
  const linkRef = useRef<HTMLAnchorElement>(null);

  return (
    <CommandItem
      keywords={[item.description]}
      onSelect={() => linkRef.current?.click()}
      value={`${item.label} ${item.description}`}
    >
      <Link className="flex min-w-0 flex-1 items-center gap-3" href={item.href} onClick={onNavigate} ref={linkRef}>
        <item.icon className="size-4 text-muted-foreground" aria-hidden="true" />
        <span>{item.label}</span>
        <span className="ml-auto truncate text-xs text-muted-foreground">{item.description}</span>
      </Link>
    </CommandItem>
  );
}

function AccountMenu({
  role,
  viewer,
}: {
  role: WorkspaceRole;
  viewer?: LayoutViewer;
}) {
  const [logout, logoutState] = useLogoutMutation();

  async function handleLogout() {
    try {
      await logout().unwrap();
    } finally {
      clearAccessToken();
      window.location.replace("/login");
    }
  }

  return (
    <>
      <Button aria-label="任务通知" className="hidden sm:inline-flex" size="icon" variant="ghost">
        <Bell aria-hidden="true" />
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button className="gap-2 px-1.5" variant="ghost">
            <Avatar size="sm">
              <AvatarFallback>{(viewer?.displayName ?? "L").slice(0, 1).toUpperCase()}</AvatarFallback>
            </Avatar>
            <span className="hidden max-w-28 truncate text-sm md:block">{viewer?.displayName ?? "账户"}</span>
            <ChevronDown className="hidden size-3.5 text-muted-foreground sm:block" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel>
            <span className="block text-foreground">{viewer?.displayName ?? "Lanverse"}</span>
            <span className="mt-0.5 block font-normal">{viewer?.workspaceName ?? "工作空间"} · {roleLabels[role]}</span>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild><Link href="/workspaces"><UserRound aria-hidden="true" />账户与空间</Link></DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={logoutState.isLoading}
            onSelect={(event) => {
              event.preventDefault();
              void handleLogout();
            }}
            variant="destructive"
          >
            <LogOut aria-hidden="true" />退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}

export function BasicHeader({
  active,
  authState,
  currentStep,
  projectName,
  role,
  viewer,
}: {
  active?: StudioNavigation;
  authState: LayoutAuthState;
  currentStep?: number;
  projectName?: string;
  role?: WorkspaceRole;
  viewer?: LayoutViewer;
}) {
  const showAppNavigation = authState !== "anonymous";

  return (
    <header aria-label="Lanverse 全局页眉" className="basic-layout__header bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85">
      <div className="basic-layout__primary-header">
        <LayoutContainer className="flex h-full items-center gap-6">
          <StudioBrand size="l" />
          <div className="hidden min-w-56 md:block">
            {authState === "loading" || (authState === "authenticated" && !role) ? (
              <NavigationPlaceholder />
            ) : showAppNavigation ? (
              <StudioNavigationMenu active={active} role={role} />
            ) : (
              <div aria-hidden="true" className="h-8 w-56" />
            )}
          </div>

          <div className="ml-auto flex min-w-0 items-center gap-1.5">
            {authState === "loading" || (authState === "authenticated" && !role) ? (
              <div aria-hidden="true" className="hidden h-8 w-64 bg-muted/35 xl:block" />
            ) : role ? (
              <GlobalSearch role={role} />
            ) : null}
            <ThemeToggle />
            {authState === "anonymous" ? <Button asChild><Link href="/login">登录</Link></Button> : null}
            {authState === "authenticated" && role ? (
              <AccountMenu role={role} viewer={viewer} />
            ) : null}
          </div>
        </LayoutContainer>
      </div>

      {authState === "anonymous" ? null : (
        <div className="basic-layout__mobile-nav flex items-center overflow-x-auto bg-muted/35 px-5 md:hidden">
          {authState === "loading" || (authState === "authenticated" && !role) ? (
            <NavigationPlaceholder mobile />
          ) : showAppNavigation ? (
            <StudioNavigationMenu active={active} mobile role={role} />
          ) : (
            <div aria-hidden="true" className="h-8 w-56" />
          )}
        </div>
      )}

      {projectName || typeof currentStep === "number" ? (
        <div className="basic-layout__context bg-background">
          {projectName ? (
            <LayoutContainer className="flex h-11 items-center gap-3 text-sm">
              <span className="text-muted-foreground">当前项目</span>
              <span className="font-medium">{projectName}</span>
            </LayoutContainer>
          ) : null}
          {typeof currentStep === "number" ? <ProductionProgress currentStep={currentStep} /> : null}
        </div>
      ) : null}
    </header>
  );
}
