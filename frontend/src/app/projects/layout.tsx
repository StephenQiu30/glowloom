import Link from "next/link";
import { ThemeToggle } from "@/components/theme-toggle";

export default function ProjectsLayout({ children }: LayoutProps<"/projects">) {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      <a
        href="#main-content"
        className="sr-only rounded-md bg-background px-3 py-2 text-sm font-medium focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-10 focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none"
      >
        跳转到主要内容
      </a>
      <header className="px-6 py-6 lg:px-12">
        <div className="mx-auto flex w-full max-w-7xl items-center justify-between gap-6">
          <Link
            href="/projects"
            aria-label="Lanverse，返回项目列表"
            className="inline-flex min-w-0 items-center gap-3 rounded-md text-foreground transition-colors hover:text-foreground/70 focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none"
          >
            <span
              aria-hidden="true"
              className="flex size-9 shrink-0 items-center justify-center rounded-[11px] bg-foreground text-lg font-semibold tracking-tight text-background"
            >
              L
            </span>
            <span
              className="truncate text-[17px] font-semibold tracking-tight"
              translate="no"
            >
              Lanverse
            </span>
          </Link>
          <nav aria-label="主要导航" className="flex items-center gap-2">
            <Link
              href="/projects"
              aria-current="page"
              className="rounded-lg bg-muted px-4 py-2 text-sm font-medium transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:outline-none"
            >
              项目
            </Link>
          </nav>
          <div className="flex items-center gap-3">
            <span className="hidden text-xs font-medium tracking-wide text-muted-foreground sm:block">
              创作工作台
            </span>
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main
        id="main-content"
        className="mx-auto w-full max-w-7xl flex-1 px-6 pt-12 pb-20 lg:px-12 lg:pt-16"
      >
        {children}
      </main>
    </div>
  );
}
