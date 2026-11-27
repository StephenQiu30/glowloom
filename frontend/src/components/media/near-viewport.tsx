"use client";

import { inView } from "motion";
import { useCallback, useState, type ReactNode } from "react";

/** Keep the reserved space while deferring private reads and player mounts. */
export function NearViewport({
  children,
  placeholder,
  className,
}: {
  children: ReactNode;
  placeholder: ReactNode;
  className?: string;
}) {
  const [ready, setReady] = useState(false);
  const observe = useCallback((element: HTMLDivElement | null) => {
    if (!element) return;
    if (!window.IntersectionObserver) {
      setReady(true);
      return;
    }
    return inView(element, () => setReady(true), { margin: "160px" });
  }, []);
  return (
    <div ref={observe} className={className}>
      {ready ? children : placeholder}
    </div>
  );
}
