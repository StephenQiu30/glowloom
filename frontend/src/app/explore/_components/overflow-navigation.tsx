"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import styles from "./explore.module.css";

export function OverflowNavigation({
  label,
  children,
  primary = false,
}: {
  label: string;
  children: ReactNode;
  primary?: boolean;
}) {
  const container = useRef<HTMLDivElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({
    overflow: false,
    start: true,
    end: true,
  });

  useEffect(() => {
    const outer = container.current;
    const view = viewport.current;
    const inner = content.current;
    if (!outer || !view || !inner) return;
    const measure = () => {
      // Compare with the full row so arrows cannot sustain false overflow.
      const next = {
        overflow: inner.scrollWidth > outer.clientWidth + 1,
        start: view.scrollLeft <= 1,
        end: view.scrollLeft + view.clientWidth >= view.scrollWidth - 1,
      };
      setEdges((previous) =>
        previous.overflow === next.overflow &&
        previous.start === next.start &&
        previous.end === next.end
          ? previous
          : next,
      );
    };
    const observer = new ResizeObserver(() => {
      const selected = inner.querySelector<HTMLElement>(
        '[aria-selected="true"], [aria-checked="true"]',
      );
      if (selected) keepVisible(view, selected);
      measure();
    });
    observer.observe(outer);
    observer.observe(view);
    observer.observe(inner);
    view.addEventListener("scroll", measure, { passive: true });
    measure();
    return () => {
      observer.disconnect();
      view.removeEventListener("scroll", measure);
    };
  }, []);

  const scroll = (direction: number) => {
    const view = viewport.current;
    if (!view) return;
    view.scrollBy({
      left: direction * view.clientWidth * 0.8,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth",
    });
  };

  return (
    <div
      className={styles.navigation}
      data-explore-navigation={primary || undefined}
      ref={container}
    >
      {edges.overflow && (
        <Button
          variant="ghost"
          size="icon"
          className={styles.arrow}
          aria-label={"向左滚动" + label}
          disabled={edges.start}
          onClick={() => scroll(-1)}
        >
          <ChevronLeft />
        </Button>
      )}
      <div
        className={styles.viewport}
        ref={viewport}
        onFocusCapture={(event) => {
          const view = viewport.current;
          if (!view) return;
          keepVisible(view, event.target);
        }}
      >
        <div className={styles.navigationContent} ref={content}>
          {children}
        </div>
      </div>
      {edges.overflow && (
        <Button
          variant="ghost"
          size="icon"
          className={styles.arrow}
          aria-label={"向右滚动" + label}
          disabled={edges.end}
          onClick={() => scroll(1)}
        >
          <ChevronRight />
        </Button>
      )}
    </div>
  );
}

function keepVisible(view: HTMLElement, target: HTMLElement) {
  const item = target.getBoundingClientRect();
  const bounds = view.getBoundingClientRect();
  if (item.left < bounds.left) view.scrollLeft -= bounds.left - item.left;
  else if (item.right > bounds.right)
    view.scrollLeft += item.right - bounds.right;
}
