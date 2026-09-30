"use client";

import { useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  ArrowUpRight,
  BookOpen,
  ChevronRight,
  Clapperboard,
  FolderOpen,
  House,
  Images,
  ListChecks,
  Menu,
  PanelsTopLeft,
  Plus,
  Settings2,
  CircleHelp,
  UserRound,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogClose,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import { projects, episodes } from "./data";
import { projectSections } from "./routes";
import { cn } from "@/lib/utils";

const mainLinks = [
  { href: "/", label: "首页", icon: House },
  { href: "/projects", label: "项目", icon: FolderOpen },
  { href: "/assets", label: "资产", icon: Images },
  { href: "/tasks", label: "任务中心", icon: ListChecks },
] as const;
const adminSections = [
  ["users", "用户与权限"],
  ["providers", "供应商"],
  ["models", "模型与价格"],
  ["audit", "审计记录"],
  ["health", "依赖状态"],
] as const;

function WorkspaceNavigation({
  pathname,
  onNavigate,
}: {
  pathname: string;
  onNavigate?: () => void;
}) {
  const router = useRouter();
  const project = pathname.startsWith("/projects/")
    ? projects.find((item) => item.id === pathname.split("/")[2])
    : undefined;
  const episodeId = episodes.some((item) => item.id === pathname.split("/")[4])
    ? pathname.split("/")[4]
    : "ep-01";
  const admin = pathname.startsWith("/admin");
  const secondaryLinks = project
    ? projectSections.map(([path, label]) => ({
        href:
          path === "canvas"
            ? "/canvas"
            : `/projects/${project.id}${path ? `/${path.replace("ep-01", episodeId)}` : ""}`,
        label,
      }))
    : admin
      ? adminSections.map(([path, label]) => ({
          href: `/admin/${path}`,
          label,
        }))
      : [];
  return (
    <div className="flex h-full flex-col gap-6 px-4 pt-7 pb-4">
      <Link
        href="/"
        onClick={onNavigate}
        aria-label="Lanverse，返回首页"
        className="flex w-fit items-center gap-2.5 rounded-lg px-2 text-xl font-semibold tracking-tight outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Clapperboard aria-hidden="true" className="size-7" />
        Lanverse
      </Link>
      <div className="flex flex-col gap-2">
        <Button asChild size="lg" className="h-10 w-full justify-start px-3">
          <Link href="/projects?create=true" onClick={onNavigate}>
            <Plus data-icon="inline-start" />
            新建项目
          </Link>
        </Button>
        <Button variant="ghost" asChild className="h-10 justify-start px-3">
          <Link href="/canvas" onClick={onNavigate}>
            <PanelsTopLeft data-icon="inline-start" />
            创作画布
            <ArrowUpRight
              aria-hidden="true"
              className="ml-auto size-3.5 text-muted-foreground"
            />
          </Link>
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <nav aria-label="主要导航" className="flex flex-col gap-1">
          {mainLinks.map(({ href, label, icon: Icon }) => {
            const active =
              href === "/" ? pathname === "/" : pathname.startsWith(href);
            return (
              <Link
                key={href}
                href={href}
                onClick={onNavigate}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex h-10 items-center gap-3 rounded-lg px-3 text-sm text-muted-foreground transition-colors outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
                  active && "bg-sidebar-accent font-medium text-foreground",
                )}
              >
                <Icon aria-hidden="true" className="size-4.5" />
                {label}
              </Link>
            );
          })}
        </nav>
        {secondaryLinks.length > 0 ? (
          <div className="mt-7 flex flex-col gap-3">
            <p className="px-3 text-xs text-muted-foreground">
              {project ? "项目流程 · 样例预览" : "内部管理 · 样例预览"}
            </p>
            {project ? (
              <div className="flex flex-col gap-2 px-2">
                <Select
                  value={project.id}
                  onValueChange={(id) => {
                    onNavigate?.();
                    router.push(`/projects/${id}`);
                  }}
                >
                  <SelectTrigger aria-label="切换样例项目" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {projects.map((item) => (
                        <SelectItem key={item.id} value={item.id}>
                          {item.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Select
                  value={episodeId}
                  onValueChange={(id) => {
                    onNavigate?.();
                    router.push(`/projects/${project.id}/episodes/${id}/shots`);
                  }}
                >
                  <SelectTrigger aria-label="切换单集" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {episodes.map((item) => (
                        <SelectItem key={item.id} value={item.id}>
                          {item.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            ) : null}
            <nav
              aria-label={project ? "项目导航" : "管理导航"}
              className="flex flex-col gap-1"
            >
              {secondaryLinks.map(({ href, label }) => (
                <Link
                  key={href}
                  href={href}
                  onClick={onNavigate}
                  aria-current={pathname === href ? "page" : undefined}
                  className={cn(
                    "rounded-lg px-3 py-2 text-sm text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
                    pathname === href && "bg-sidebar-accent text-foreground",
                  )}
                >
                  {label}
                </Link>
              ))}
            </nav>
          </div>
        ) : (
          <div className="mt-7 flex flex-col gap-1">
            <p className="mb-2 px-3 text-xs text-muted-foreground">创作空间</p>
            <Link
              href="/#guides"
              onClick={onNavigate}
              className="flex h-10 items-center gap-3 rounded-lg px-3 text-sm text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              <BookOpen aria-hidden="true" className="size-4.5" />
              创作指南
              <Badge variant="secondary" className="ml-auto text-primary">
                入门
              </Badge>
            </Link>
            <Link
              href="/admin/providers"
              onClick={onNavigate}
              className="flex h-10 items-center gap-3 rounded-lg px-3 text-sm text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Settings2 aria-hidden="true" className="size-4.5" />
              服务管理
            </Link>
          </div>
        )}
      </div>
      <div className="flex flex-col gap-3">
        <Link
          href="/#guides"
          onClick={onNavigate}
          className="group relative hidden aspect-[1.8] overflow-hidden rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring lg:block"
        >
          <Image
            src="/studio/motion-studio.png"
            alt="雾中森林，创作指南示意图"
            fill
            sizes="208px"
            className="object-cover transition-transform duration-300 group-hover:scale-105 motion-reduce:transform-none motion-reduce:transition-none"
          />
          <div className="absolute inset-0 bg-linear-to-t from-black/90 via-black/20 to-transparent" />
          <div className="absolute inset-x-4 bottom-4 text-white">
            <p className="text-base font-medium">让故事成为画面</p>
            <p className="mt-1 text-xs text-white/70">从你的第一张画布开始</p>
          </div>
        </Link>
        <Link
          href="/account"
          onClick={onNavigate}
          className="flex h-9 items-center gap-3 rounded-lg px-3 text-sm text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <UserRound aria-hidden="true" className="size-4" />
          账号与设置
          <ChevronRight aria-hidden="true" className="ml-auto size-4" />
        </Link>
      </div>
    </div>
  );
}

export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  if (/^\/projects\/[^/]+\/canvas$/.test(pathname)) return <>{children}</>;
  const title =
    pathname === "/"
      ? "创作工作台"
      : pathname.startsWith("/projects")
        ? "项目空间"
        : pathname.startsWith("/assets")
          ? "资产库"
          : pathname.startsWith("/tasks")
            ? "任务中心"
            : pathname.startsWith("/admin")
              ? "服务管理"
              : "账号与设置";
  return (
    <div className="min-h-dvh bg-background text-foreground">
      <a
        href="#main-content"
        className="sr-only rounded-lg bg-popover p-3 focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-50"
      >
        跳转到主要内容
      </a>
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 bg-sidebar lg:block">
        <WorkspaceNavigation pathname={pathname} />
      </aside>
      <div className="min-w-0 lg:pl-60">
        <header className="flex h-16 items-center justify-between gap-3 px-4 sm:px-7 lg:px-10">
          <div className="flex min-w-0 items-center gap-3">
            <Dialog open={menuOpen} onOpenChange={setMenuOpen}>
              <DialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="打开导航菜单"
                  className="lg:hidden"
                >
                  <Menu />
                </Button>
              </DialogTrigger>
              <DialogContent className="inset-y-0 left-0 h-dvh w-[min(20rem,85vw)] translate-x-0 translate-y-0 gap-0 rounded-none bg-sidebar p-0 sm:max-w-none">
                <DialogHeader className="sr-only">
                  <DialogTitle>工作台导航</DialogTitle>
                  <DialogDescription>
                    访问项目、画布、资产和任务。
                  </DialogDescription>
                </DialogHeader>
                <WorkspaceNavigation
                  pathname={pathname}
                  onNavigate={() => setMenuOpen(false)}
                />
              </DialogContent>
            </Dialog>
            <span className="truncate text-sm text-muted-foreground">
              {title}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="secondary" className="rounded-full">
                  <CircleHelp data-icon="inline-start" />
                  <span className="hidden sm:inline">使用指南</span>
                  <span className="sr-only sm:hidden">使用指南</span>
                </Button>
              </DialogTrigger>
              <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>开始你的创作</DialogTitle>
                  <DialogDescription>
                    新建项目，确定画幅与风格，再进入画布组织故事和已有素材。
                  </DialogDescription>
                </DialogHeader>
                <ol className="flex list-decimal flex-col gap-3 pl-5 text-sm leading-6 text-muted-foreground">
                  <li>在项目空间创建或打开项目，进入创作画布。</li>
                  <li>从资产库选择项目，查看已有图片、视频和音频。</li>
                  <li>生成服务与任务中心中的样例会显示准备或预览状态。</li>
                </ol>
                <DialogClose asChild>
                  <Button asChild>
                    <Link href="/projects?create=true">
                      创建第一个项目
                      <ArrowUpRight data-icon="inline-end" />
                    </Link>
                  </Button>
                </DialogClose>
              </DialogContent>
            </Dialog>
            <ThemeToggle />
            <Button
              variant="secondary"
              size="icon"
              asChild
              className="rounded-full"
            >
              <Link href="/account" aria-label="账号设置">
                <UserRound />
              </Link>
            </Button>
          </div>
        </header>
        <main
          id="main-content"
          tabIndex={-1}
          className="mx-auto max-w-[1800px] px-4 pb-12 outline-none sm:px-7 lg:px-10"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
