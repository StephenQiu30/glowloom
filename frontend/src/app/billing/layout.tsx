"use client";

import Link from "next/link";
import { useSelectedLayoutSegment } from "next/navigation";
import { useCallback, type ReactNode } from "react";
import { billingSections } from "@/components/billing/billing-sections";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { Button } from "@/components/ui/button";

export default function BillingLayout({ children }: { children: ReactNode }) {
  const section = useSelectedLayoutSegment() ?? "overview";
  const revealActiveLink = useCallback((link: HTMLAnchorElement | null) => {
    const nav = link?.parentElement;
    if (!link || !nav) return;
    const reveal = () => {
      const bounds = nav.getBoundingClientRect();
      const active = link.getBoundingClientRect();
      if (active.left < bounds.left)
        nav.scrollLeft += active.left - bounds.left;
      if (active.right > bounds.right)
        nav.scrollLeft += active.right - bounds.right;
    };
    reveal();
    const observer = new ResizeObserver(reveal);
    observer.observe(nav);
    return () => observer.disconnect();
  }, []);

  return (
    <>
      <PageToolbar inHeader>
        <nav aria-label="账务导航" className="billing-navigation">
          {Object.entries(billingSections).map(([key, item]) => (
            <Button
              key={key}
              asChild
              size="workspace"
              variant={section === key ? "secondary" : "ghost"}
            >
              <Link
                href={item.href}
                aria-current={section === key ? "page" : undefined}
                aria-label={item.title}
                ref={section === key ? revealActiveLink : undefined}
              >
                {item.label}
              </Link>
            </Button>
          ))}
        </nav>
      </PageToolbar>
      {children}
    </>
  );
}
