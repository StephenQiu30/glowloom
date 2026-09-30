"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Bell,
  ChevronRight,
  Folder,
  ListChecks,
  Settings2,
  UserRound,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
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
import { projects, tasks, episodes } from "./data";
import { projectSections } from "./routes";
import { cn } from "@/lib/utils";

const adminSections = [
  ["users", "用户与权限"],
  ["providers", "供应商"],
  ["models", "模型与价格"],
  ["audit", "审计记录"],
  ["health", "依赖状态"],
] as const;
export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  // 正式项目和画布提供真实会话、导航与各自的工作区。
  if (pathname === "/projects" || /^\/projects\/[^/]+\/canvas$/.test(pathname))
    return <>{children}</>;
  const episodeId = episodes.some((e) => e.id === pathname.split("/")[4])
    ? pathname.split("/")[4]
    : "ep-01";
  const projectId = pathname.split("/")[2];
  const project = pathname.startsWith("/projects/")
    ? projects.find((p) => p.id === projectId)
    : undefined;
  const admin = pathname.startsWith("/admin");
  const nav = project
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
    <div className="min-h-screen bg-background text-foreground">
      <a
        href="#main-content"
        className="sr-only rounded bg-background p-3 focus:not-sr-only focus:absolute focus:z-50"
      >
        跳转到主要内容
      </a>
      <header className="flex flex-wrap items-center justify-between gap-4 px-6 py-5 lg:px-9">
        <div className="flex items-center gap-8">
          <Link
            href="/projects"
            aria-label="Lanverse，返回项目列表"
            className="flex items-center gap-2.5 rounded-md focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <span
              aria-hidden="true"
              className="flex size-8 items-center justify-center rounded-xl bg-foreground font-semibold text-background"
            >
              L
            </span>
            <span className="font-semibold tracking-tight">Lanverse</span>
          </Link>
          <nav aria-label="主要导航" className="flex gap-1">
            {[
              ["/projects", "项目", Folder],
              ["/canvas", "画布", Folder],
              ["/tasks", "任务", ListChecks],
              ["/admin/users", "管理", Settings2],
            ].map(([href, label, Icon]) => {
              const active = pathname.startsWith(
                String(href).split("/").slice(0, 2).join("/"),
              );
              const NavIcon = Icon as typeof Folder;
              return (
                <Link
                  key={String(href)}
                  href={String(href)}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-blue-500",
                    active && "bg-muted font-medium text-foreground",
                  )}
                >
                  <NavIcon aria-hidden="true" className="size-4" />
                  {String(label)}
                </Link>
              );
            })}
          </nav>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="hidden sm:inline-flex">
            演示模式
          </Badge>
          <Dialog>
            <DialogTrigger asChild>
              <Button variant="ghost" size="icon" aria-label="通知">
                <Bell />
              </Button>
            </DialogTrigger>
            <DialogContent className="sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>工作台通知</DialogTitle>
                <DialogDescription>
                  样例通知，尚未接入消息服务。
                </DialogDescription>
              </DialogHeader>
              <ul className="space-y-4">
                {tasks.slice(0, 3).map((t) => (
                  <li key={t.id}>
                    <Link
                      className="block rounded-lg p-3 hover:bg-muted"
                      href={`/tasks/${t.id}`}
                    >
                      <p className="font-medium">
                        {t.title} · {t.label}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t.time}
                      </p>
                    </Link>
                  </li>
                ))}
              </ul>
            </DialogContent>
          </Dialog>
          <ThemeToggle />
          <Button variant="ghost" size="icon" asChild>
            <Link href="/account" aria-label="账号设置">
              <UserRound />
            </Link>
          </Button>
        </div>
      </header>
      <div
        className={cn(
          "mx-auto max-w-[1600px] px-6 pb-16 lg:px-9",
          nav.length > 0 &&
            "grid gap-8 lg:grid-cols-[180px_minmax(0,1fr)] lg:gap-10",
        )}
      >
        {nav.length > 0 && (
          <aside className="pt-5">
            {project ? (
              <div className="mb-5 space-y-3">
                <Select
                  value={project.id}
                  onValueChange={(id) => router.push(`/projects/${id}`)}
                >
                  <SelectTrigger aria-label="切换项目" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {projects.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Select
                  value={episodeId}
                  onValueChange={(id) =>
                    router.push(`/projects/${project.id}/episodes/${id}/shots`)
                  }
                >
                  <SelectTrigger aria-label="切换单集" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {episodes.map((e) => (
                        <SelectItem key={e.id} value={e.id}>
                          {e.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            ) : (
              <p className="mb-5 px-3 text-sm font-semibold">内部管理</p>
            )}
            <nav
              aria-label={project ? "项目导航" : "管理导航"}
              className="flex flex-wrap gap-1 lg:flex-col"
            >
              {nav.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={
                    pathname === item.href ||
                    (item.label === "分镜" &&
                      pathname.startsWith(`${item.href}/`))
                      ? "page"
                      : undefined
                  }
                  className={cn(
                    "rounded-lg px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-blue-500",
                    (pathname === item.href ||
                      (item.label === "分镜" &&
                        pathname.startsWith(`${item.href}/`))) &&
                      "bg-muted font-medium text-foreground",
                  )}
                >
                  {item.label}
                </Link>
              ))}
            </nav>
            {project && (
              <p className="mt-9 hidden px-3 text-xs leading-6 text-muted-foreground lg:block">
                从故事到镜头
                <br />
                让每一步创作保持连贯。
              </p>
            )}
          </aside>
        )}
        <main id="main-content" className="min-w-0 pt-5">
          <div className="mb-7 flex items-center gap-2 text-xs text-muted-foreground">
            <Link href="/projects">工作台</Link>
            <ChevronRight aria-hidden="true" className="size-3" />
            <span>
              {project?.name ??
                (admin
                  ? "内部管理"
                  : pathname.startsWith("/tasks")
                    ? "任务中心"
                    : "我的项目")}
            </span>
          </div>
          {children}
        </main>
      </div>
    </div>
  );
}
