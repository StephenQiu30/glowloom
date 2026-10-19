"use client";

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from "react";
import { cn } from "cn";
import type { PageSearch } from "./product-search";
import type { Screen } from "./routes";

export type PageOptions = {
  owner: symbol;
  screen: Screen;
  search?: PageSearch;
};

// Only the stable setter is shared with pages. Header updates do not subscribe
// every page to shell state or rerender the editor on each search keystroke.
export const PageOptionsContext = createContext<Dispatch<
  SetStateAction<PageOptions | null>
> | null>(null);

export function PageLayout({
  screen,
  children,
  fullBleed = false,
  search,
}: {
  screen: Screen;
  children: ReactNode;
  fullBleed?: boolean;
  search?: PageSearch;
}) {
  const setOptions = useContext(PageOptionsContext);
  const owner = useRef(Symbol("page"));
  useEffect(() => {
    if (!setOptions) return;
    const id = owner.current;
    setOptions({ owner: id, screen, search });
    return () => {
      setOptions((current) => (current?.owner === id ? null : current));
    };
  }, [screen, search, setOptions]);

  return (
    <div
      data-page={screen}
      data-full-bleed={fullBleed}
      className={cn("product-page", !fullBleed && "workspace-page")}
    >
      {children}
    </div>
  );
}
