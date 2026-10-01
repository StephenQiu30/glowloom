"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Clapperboard,
  Home,
  Image,
  Layers,
  ListChecks,
  Menu,
  Plus,
  Settings2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

const navigation = [
  { href: "/", label: "首页", icon: Home },
  { href: "/create", label: "创作", icon: Clapperboard },
  { href: "/projects", label: "项目", icon: Layers },
  { href: "/assets", label: "资产", icon: Image },
  { href: "/tasks", label: "任务", icon: ListChecks },
  { href: "/settings", label: "模型配置", icon: Settings2 },
] as const;

export function WorkspaceShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const desktop = window.matchMedia("(min-width: 768px)");
    const closeOnDesktop = () => {
      if (desktop.matches) setOpen(false);
    };
    desktop.addEventListener("change", closeOnDesktop);
    return () => desktop.removeEventListener("change", closeOnDesktop);
  }, []);
  return (
    <div className="min-h-dvh bg-background">
      <a
        href="#workspace-main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-background focus:p-3"
      >
        跳至内容
      </a>
      <header className="flex h-16 items-center justify-between px-5 md:hidden">
        <Link href="/" className="font-semibold tracking-tight">
          LANVERSE
        </Link>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="打开导航">
              <Menu />
            </Button>
          </DialogTrigger>
          <DialogContent className="top-0 left-0 flex h-dvh w-56 max-w-56 translate-x-0 translate-y-0 flex-col rounded-none bg-sidebar px-4 py-7 sm:max-w-56">
            <DialogTitle className="sr-only">工作区导航</DialogTitle>
            <DialogDescription className="sr-only">
              选择创作、项目、资产、任务或模型配置。
            </DialogDescription>
            <WorkspaceNavigation
              pathname={pathname}
              close={() => setOpen(false)}
            />
          </DialogContent>
        </Dialog>
      </header>
      <aside
        id="workspace-navigation"
        className="fixed inset-y-0 left-0 z-30 hidden w-56 flex-col bg-sidebar px-4 py-7 md:flex"
      >
        <WorkspaceNavigation pathname={pathname} close={() => setOpen(false)} />
      </aside>
      <main
        id="workspace-main"
        className="mx-auto min-h-dvh px-5 py-8 md:ml-56 md:px-10 md:py-12 lg:px-16"
      >
        {children}
      </main>
    </div>
  );
}

function WorkspaceNavigation({
  pathname,
  close,
}: {
  pathname: string;
  close: () => void;
}) {
  return (
    <>
      <Link
        href="/"
        className="mb-10 px-3 text-lg font-semibold tracking-tighter"
        onClick={close}
      >
        LANVERSE
        <span className="ml-2 text-xs font-normal tracking-normal text-muted-foreground">
          Studio
        </span>
      </Link>
      <Button asChild className="mb-6 justify-start">
        <Link href="/projects?create=true" onClick={close}>
          <Plus />
          新建项目
        </Link>
      </Button>
      <nav aria-label="工作区" className="flex flex-col gap-1">
        {navigation.map(({ href, label, icon: Icon }) => {
          const active =
            href === "/" ? pathname === href : pathname.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              aria-current={active ? "page" : undefined}
              onClick={close}
              className={`flex items-center gap-3 rounded-lg px-3 py-3 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 ${active ? "bg-sidebar-accent font-medium text-foreground" : "text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"}`}
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </Link>
          );
        })}
      </nav>
      <div className="mt-auto flex items-center justify-between px-3 pt-8">
        <Link
          href="/licenses"
          className="text-xs text-muted-foreground"
          onClick={close}
        >
          关于与许可
        </Link>
        <ThemeToggle />
      </div>
    </>
  );
}
