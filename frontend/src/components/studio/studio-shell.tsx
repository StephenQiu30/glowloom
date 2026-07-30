"use client";

import {
  ArrowRight,
  Box,
  Check,
  Folder,
  Home,
  LogOut,
  Settings,
  ShieldCheck,
  Sparkles,
  WandSparkles,
} from "lucide-react";
import Link from "next/link";
import { type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { clearAccessToken } from "@/lib/auth-session";
import { cn } from "@/lib/class-names";
import { useLogoutMutation } from "@/lib/server-state";

const productionStages = ["剧本", "资产", "分镜", "生成", "审核", "交付"];

export type StudioNavigation =
  | "create"
  | "projects"
  | "assets"
  | "governance"
  | "settings";

export function StudioBrand() {
  return (
    <Link className="flex items-center gap-2 px-1 font-semibold tracking-tight" href="/">
      <span className="grid size-8 place-items-center rounded-xl bg-[#09a6bc] text-white shadow-sm shadow-cyan-500/15">
        <Sparkles className="size-[18px]" strokeWidth={2.3} aria-hidden="true" />
      </span>
      <span className="hidden text-[17px] lg:inline">Lanverse</span>
    </Link>
  );
}

function ProductionProgress({ currentStep }: { currentStep: number }) {
  return (
    <ol className="hidden min-w-0 flex-1 items-center justify-center gap-2 xl:flex" aria-label="制作进度">
      {productionStages.map((stage, index) => (
        <li className="flex items-center gap-2" key={stage}>
          <span className={cn(
            "grid size-6 place-items-center rounded-full border text-xs font-medium",
            index <= currentStep ? "border-[#079db3] bg-[#079db3] text-white" : "border-slate-300 text-slate-500",
          )}>{index < currentStep ? <Check className="size-3.5" aria-hidden="true" /> : index + 1}</span>
          <span className={cn("text-sm", index === currentStep ? "font-medium text-slate-900" : "text-slate-500")}>{stage}</span>
          {index < productionStages.length - 1 ? <span className="mx-1 h-px w-8 bg-slate-200 2xl:w-12" /> : null}
        </li>
      ))}
    </ol>
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
  const [logout, logoutState] = useLogoutMutation();
  const navItems = [
    { id: "create" as const, label: "创作", icon: Home, href: "/" },
    { id: "projects" as const, label: "项目", icon: Folder, href: "/projects" },
    { id: "assets" as const, label: "资产", icon: Box, href: "/studio" },
    {
      id: "governance" as const,
      label: "治理",
      icon: ShieldCheck,
      href: "/governance",
    },
  ];

  async function handleLogout() {
    try {
      await logout().unwrap();
    } finally {
      clearAccessToken();
      window.location.replace("/login");
    }
  }

  return (
    <main className="min-h-screen bg-[#fbfcfd] text-slate-950">
      <aside className="fixed inset-y-0 left-0 z-30 flex w-[76px] flex-col border-r border-slate-200/80 bg-white px-3 py-5 lg:w-[156px]">
        <StudioBrand />
        <div className="mt-7">
          <Button asChild className="h-10 w-full bg-[#079db3] text-white hover:bg-[#078da0]">
            <Link href="/projects"><Folder aria-hidden="true" /><span className="hidden lg:inline">项目库</span></Link>
          </Button>
        </div>
        <nav className="mt-7 grid gap-2" aria-label="主导航">
          {navItems.map((item) => (
            <Link className={cn(
              "flex h-12 items-center justify-center gap-3 rounded-xl text-sm transition-colors lg:justify-start lg:px-4",
              active === item.id ? "bg-slate-100 text-[#078fa5]" : "text-slate-600 hover:bg-slate-50 hover:text-slate-950",
            )} href={item.href} key={item.id}>
              <item.icon className="size-5" strokeWidth={1.8} aria-hidden="true" />
              <span className="hidden lg:inline">{item.label}</span>
            </Link>
          ))}
        </nav>
        <div className="mt-auto grid gap-2">
          <Link className={cn(
            "flex items-center justify-center gap-2 rounded-xl p-2 transition-colors hover:bg-slate-50 lg:justify-start",
            active === "settings" && "bg-slate-100",
          )} href="/workspaces">
            <span className="grid size-9 shrink-0 place-items-center rounded-full border border-cyan-100 bg-cyan-50 text-sm font-semibold text-[#087f91]" aria-hidden="true">
              {(viewer?.displayName ?? "账户").slice(0, 1).toUpperCase()}
            </span>
            <span className="hidden min-w-0 lg:block">
              <span className="block truncate text-sm font-medium">{viewer?.displayName ?? "账户"}</span>
              <span className="block truncate text-xs text-slate-400">{viewer?.workspaceName ?? "工作空间"}</span>
            </span>
            <Settings className="ml-auto hidden size-4 text-slate-400 lg:block" aria-hidden="true" />
          </Link>
          {viewer ? (
            <Button
              aria-label="退出登录"
              disabled={logoutState.isLoading}
              onClick={handleLogout}
              size="sm"
              variant="ghost"
            >
              <LogOut aria-hidden="true" />
              <span className="hidden lg:inline">退出登录</span>
            </Button>
          ) : null}
        </div>
      </aside>

      <div className="min-h-screen pl-[76px] lg:pl-[156px]">
        <header className="sticky top-0 z-20 flex h-[76px] items-center gap-5 border-b border-slate-200/70 bg-white/95 px-5 backdrop-blur md:px-8">
          {projectName ? (
            <div className="shrink-0 text-sm font-medium">{projectName}</div>
          ) : (
            <div className="flex shrink-0 items-center gap-2 whitespace-nowrap text-sm font-medium"><WandSparkles className="size-4 text-[#079db3]" aria-hidden="true" />AI 漫剧创作台</div>
          )}
          {typeof currentStep === "number" ? <ProductionProgress currentStep={currentStep} /> : <div className="flex-1" />}
          {topAction ?? (
            <Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/projects">查看项目<ArrowRight aria-hidden="true" /></Link></Button>
          )}
        </header>
        {children}
      </div>
    </main>
  );
}
