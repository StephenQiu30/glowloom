"use client";

import Link from "next/link";
import type { LucideIcon } from "lucide-react";
import { useLayoutEffect, useState, type ReactNode } from "react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { PageToolbar } from "./page-toolbar";

function reveal(view: HTMLElement, item: Element) {
  const bounds = view.getBoundingClientRect();
  const active = item.getBoundingClientRect();
  if (active.left < bounds.left) view.scrollLeft += active.left - bounds.left;
  else if (active.right > bounds.right)
    view.scrollLeft += active.right - bounds.right;
}

/** Scroll only the page navigation. The account controls and page scroll stay put. */
export function HeaderNavigation({
  label,
  active,
  children,
  className,
}: {
  className?: string;
  label: string;
  active: string;
  children: ReactNode;
}) {
  const [viewport, setViewport] = useState<HTMLElement | null>(null);
  useLayoutEffect(() => {
    if (!viewport) return;
    const revealCurrent = () => {
      const current = viewport.querySelector(
        '[aria-current="page"], [aria-selected="true"]',
      );
      if (current) reveal(viewport, current);
    };
    revealCurrent();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(revealCurrent);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, [active, viewport]);
  return (
    <PageToolbar inHeader>
      <nav
        aria-label={label}
        className={cn("header-navigation", className)}
        ref={setViewport}
        onFocusCapture={(event) => reveal(event.currentTarget, event.target)}
      >
        {children}
      </nav>
    </PageToolbar>
  );
}

export function HeaderLinks({
  label,
  active,
  links,
  className,
}: {
  className?: string;
  label: string;
  active: string;
  links: readonly {
    id: string;
    href: string;
    label: string;
    title?: string;
    icon?: LucideIcon;
  }[];
}) {
  return (
    <HeaderNavigation label={label} active={active} className={className}>
      {links.map(({ id, href, label, title, icon: Icon }) => (
        <Button
          key={id}
          variant={id === active ? "secondary" : "ghost"}
          size="workspace"
          asChild
        >
          <Link
            href={href}
            aria-label={title}
            aria-current={id === active ? "page" : undefined}
          >
            {Icon ? <Icon aria-hidden="true" /> : null}
            {label}
          </Link>
        </Button>
      ))}
    </HeaderNavigation>
  );
}
