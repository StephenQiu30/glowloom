"use client";

import {
  Bell,
  Check,
  ChevronDown,
  Folder,
  Home,
  LogOut,
  Search,
  Settings,
  ShieldCheck,
  SquareStack,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { type ReactNode } from "react";

import { StudioBrand } from "@/components/studio/studio-brand";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
} from "@/components/ui/navigation-menu";
import { Separator } from "@/components/ui/separator";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  canAccessPage,
  type StudioNavigation,
  visiblePrimaryNavigation,
  type WorkspaceRole,
} from "@/lib/access-control";
import { clearAccessToken } from "@/lib/auth-session";
import { cn } from "@/lib/class-names";
import { useLogoutMutation, useMeQuery } from "@/lib/server-state";

export type { StudioNavigation } from "@/lib/access-control";

const productionStages = ["剧本", "资产", "分镜", "生成", "审核", "交付"];

const navigationItems: Array<{
  id: StudioNavigation;
  label: string;
  description: string;
  href: string;
  icon: typeof Home;
}> = [
  { id: "create", label: "首页", description: "继续当前制作", href: "/", icon: Home },
  { id: "projects", label: "项目", description: "项目与单集", href: "/projects", icon: Folder },
  { id: "assets", label: "资产", description: "角色、场景与版本", href: "/studio", icon: SquareStack },
  { id: "governance", label: "治理", description: "授权与审计", href: "/governance", icon: ShieldCheck },
  { id: "settings", label: "空间", description: "账户与工作空间", href: "/workspaces", icon: Settings },
];

const roleLabels: Record<WorkspaceRole, string> = {
  owner: "所有者",
  editor: "编辑者",
  viewer: "查看者",
};

function ProductionProgress({ currentStep }: { currentStep: number }) {
  return (
    <div className="border-b bg-background" aria-label="制作进度">
      <div className="mx-auto flex min-h-12 max-w-[1440px] items-center gap-4 overflow-x-auto px-5 md:px-8">
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
      </div>
    </div>
  );
}

function StudioNavigationMenu({
  active,
  role,
  mobile = false,
}: {
  active: StudioNavigation;
  role?: WorkspaceRole;
  mobile?: boolean;
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
                "h-8 px-2.5 py-1.5",
                active === item.id && "bg-muted font-medium",
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

function GlobalSearch({ role }: { role: WorkspaceRole }) {
  const visibleItems = navigationItems.filter((item) => canAccessPage(role, item.id));

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button className="hidden w-64 justify-start text-muted-foreground lg:flex" variant="outline">
          <Search aria-hidden="true" />
          <span>搜索或执行命令…</span>
          <kbd className="ml-auto font-mono text-[11px] text-muted-foreground">⌘K</kbd>
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg p-0">
        <DialogHeader className="border-b p-4 pb-3">
          <DialogTitle>前往 Lanverse</DialogTitle>
          <DialogDescription>搜索页面，或直接选择一个工作区入口。</DialogDescription>
        </DialogHeader>
        <div className="p-3">
          <div className="relative">
            <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input aria-label="全局搜索" className="h-10 pl-9" placeholder="搜索项目、剧本、资产或任务" autoFocus />
          </div>
          <nav className="mt-3 grid gap-1" aria-label="快速入口">
            {visibleItems.map((item) => (
              <Link className="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm hover:bg-muted" href={item.href} key={item.id}>
                <item.icon className="size-4 text-muted-foreground" aria-hidden="true" />
                <span>{item.label}</span>
                <span className="ml-auto text-xs text-muted-foreground">{item.description}</span>
              </Link>
            ))}
          </nav>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function StudioShell({
  active,
  children,
  projectName,
  currentStep,
  topAction,
  viewer,
}: {
  active: StudioNavigation;
  children: ReactNode;
  projectName?: string;
  currentStep?: number;
  topAction?: ReactNode;
  viewer?: { displayName: string; workspaceName: string };
}) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const [logout, logoutState] = useLogoutMutation();
  const role = me.data?.workspace.role;
  const resolvedViewer = viewer ?? (me.data ? {
    displayName: me.data.user.display_name?.trim() || me.data.user.email,
    workspaceName: me.data.workspace.name,
  } : undefined);

  async function handleLogout() {
    try {
      await logout().unwrap();
    } finally {
      clearAccessToken();
      window.location.replace("/login");
    }
  }

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85">
        <div className="mx-auto flex h-[72px] max-w-[1440px] items-center gap-6 px-5 md:px-8">
          <StudioBrand size="l" />
          <div className="hidden md:block">
            <StudioNavigationMenu active={active} role={role} />
          </div>

          <div className="ml-auto flex items-center gap-1.5">
            {role ? <GlobalSearch role={role} /> : null}
            {topAction ? <div className="flex items-center">{topAction}</div> : null}
            {authenticated && role ? (
              <>
                <Button aria-label="任务通知" className="hidden sm:inline-flex" size="icon" variant="ghost">
                  <Bell aria-hidden="true" />
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button className="gap-2 px-1.5" variant="ghost">
                      <Avatar size="sm">
                        <AvatarFallback>{(resolvedViewer?.displayName ?? "L").slice(0, 1).toUpperCase()}</AvatarFallback>
                      </Avatar>
                      <span className="hidden max-w-28 truncate text-sm md:block">{resolvedViewer?.displayName ?? "账户"}</span>
                      <ChevronDown className="hidden size-3.5 text-muted-foreground sm:block" aria-hidden="true" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-56">
                    <DropdownMenuLabel>
                      <span className="block text-foreground">{resolvedViewer?.displayName ?? "Lanverse"}</span>
                      <span className="mt-0.5 block font-normal">{resolvedViewer?.workspaceName ?? "工作空间"} · {roleLabels[role]}</span>
                    </DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem asChild><Link href="/workspaces"><UserRound aria-hidden="true" />账户与空间</Link></DropdownMenuItem>
                    {canAccessPage(role, "governance") ? (
                      <DropdownMenuItem asChild><Link href="/governance"><ShieldCheck aria-hidden="true" />治理与审计</Link></DropdownMenuItem>
                    ) : null}
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
            ) : null}
          </div>
        </div>

        <div className="flex h-11 items-center overflow-x-auto border-t px-5 md:hidden">
          <StudioNavigationMenu active={active} mobile role={role} />
        </div>
      </header>

      {projectName || typeof currentStep === "number" ? (
        <div className="border-b bg-background">
          {projectName ? (
            <div className="mx-auto flex min-h-11 max-w-[1440px] items-center gap-3 px-5 text-sm md:px-8">
              <span className="text-muted-foreground">当前项目</span>
              <span className="font-medium">{projectName}</span>
            </div>
          ) : null}
          {typeof currentStep === "number" ? <ProductionProgress currentStep={currentStep} /> : null}
        </div>
      ) : null}

      {children}
    </main>
  );
}
