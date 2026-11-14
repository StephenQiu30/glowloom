"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import {
  BookOpen,
  Boxes,
  GitBranch,
  ListChecks,
  FolderGit2,
  Search,
  PanelLeft,
  ChevronRight,
  ArrowUpRight,
} from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

type Item = { id: string; title: string; category: string };
const sections = [
  { href: "/", label: "知识库", icon: BookOpen },
  { href: "/skills/", label: "Skill", icon: Boxes },
  { href: "/workflows/", label: "工作流", icon: GitBranch },
  { href: "/tasks/", label: "任务", icon: ListChecks },
  { href: "/engineering/", label: "工程", icon: FolderGit2 },
];

function Directory({
  items,
  categories,
  onNavigate,
}: {
  items: Item[];
  categories: Record<string, string>;
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const [query, setQuery] = useState("");
  return (
    <nav aria-label="项目目录" className="directory">
      <Link href="/" className="brand" onClick={onNavigate}>
        <span className="brand-mark">织</span>
        <span>
          织光<span className="brand-subtitle">Workspace</span>
        </span>
      </Link>
      <Button asChild variant="outline" className="nav-search">
        <Link href="/search/" onClick={onNavigate}>
          <Search data-icon="inline-start" />
          搜索知识库<kbd>⌕</kbd>
        </Link>
      </Button>
      <div className="primary-nav">
        {sections.map(({ href, label, icon: Icon }) => (
          <Link
            href={href}
            onClick={onNavigate}
            key={href}
            className={cn("nav-link", pathname === href && "active")}
          >
            <Icon aria-hidden="true" />
            {label}
          </Link>
        ))}
      </div>
      <div className="directory-heading">
        项目文档
        <Link href="/graph/" onClick={onNavigate} aria-label="查看知识关系">
          <GitBranch />
        </Link>
      </div>
      <Input
        aria-label="筛选目录"
        placeholder="筛选文档…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="directory-filter"
      />
      <div className="directory-groups">
        {Object.entries(categories).map(([category, label]) => {
          const docs = items.filter(
            (item) =>
              item.category === category &&
              item.title.toLowerCase().includes(query.toLowerCase()),
          );
          if (!docs.length) return null;
          return (
            <details key={category} open={query ? true : undefined}>
              <summary>
                <ChevronRight aria-hidden="true" />
                {label}
                <span>{docs.length}</span>
              </summary>
              <div>
                {docs.map((item) => (
                  <Link
                    key={item.id}
                    href={`/read/${item.id}/`}
                    onClick={onNavigate}
                    className={cn(
                      "document-link",
                      pathname === `/read/${item.id}/` && "active",
                    )}
                    title={item.title}
                  >
                    {item.title}
                  </Link>
                ))}
              </div>
            </details>
          );
        })}
      </div>
      <div className="directory-footer">
        <Link href="/engineering/" onClick={onNavigate}>
          同仓知识与工程
          <ArrowUpRight />
        </Link>
      </div>
    </nav>
  );
}
export function Navigation(props: {
  items: Item[];
  categories: Record<string, string>;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <aside className="desktop-sidebar">
        <Directory {...props} />
      </aside>
      <div className="mobile-menu">
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="outline" size="icon" aria-label="打开项目目录">
              <PanelLeft />
            </Button>
          </DialogTrigger>
          <DialogContent className="mobile-directory">
            <DialogTitle className="sr-only">项目目录</DialogTitle>
            <Directory {...props} onNavigate={() => setOpen(false)} />
          </DialogContent>
        </Dialog>
      </div>
    </>
  );
}
