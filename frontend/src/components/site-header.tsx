import Link from "next/link";

import { Button } from "@/components/ui/button";
import { SessionActions } from "@/components/session-actions";

const navigation = [
  { href: "/explore", label: "探索" },
  { href: "/create", label: "创作" },
  { href: "/works", label: "作品" },
  { href: "/admin", label: "管理" },
];

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur">
      <div className="mx-auto flex min-h-16 max-w-6xl flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
        <Link
          href="/"
          className="mr-auto inline-flex items-center gap-2 font-heading font-semibold"
        >
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            T
          </span>
          <span>Thief</span>
        </Link>
        <nav aria-label="主导航" className="flex flex-wrap justify-end gap-1">
          {navigation.map((item) => (
            <Button key={item.href} asChild size="sm" variant="ghost">
              <Link href={item.href}>{item.label}</Link>
            </Button>
          ))}
          <SessionActions />
        </nav>
      </div>
    </header>
  );
}
