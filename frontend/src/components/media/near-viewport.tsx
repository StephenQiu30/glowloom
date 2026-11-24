"use client";

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
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setReady(true);
          observer.disconnect();
        }
      },
      { rootMargin: "160px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return (
    <div ref={observe} className={className}>
      {ready ? children : placeholder}
    </div>
  );
}
