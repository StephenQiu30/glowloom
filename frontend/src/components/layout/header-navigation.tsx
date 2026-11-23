"use client";

import Link from "next/link";
import { useLayoutEffect, useState, type ReactNode } from "react";
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
}: {
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
        className="header-navigation"
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
}: {
  label: string;
  active: string;
  links: readonly { id: string; href: string; label: string; title?: string }[];
}) {
  return (
    <HeaderNavigation label={label} active={active}>
      {links.map((link) => (
        <Button
          key={link.id}
          variant={link.id === active ? "secondary" : "ghost"}
          size="workspace"
          asChild
        >
          <Link
            href={link.href}
            aria-label={link.title}
            aria-current={link.id === active ? "page" : undefined}
          >
            {link.label}
          </Link>
        </Button>
      ))}
    </HeaderNavigation>
  );
}
