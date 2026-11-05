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
import { resolveAdminSection, type Screen } from "./routes";
import { ProductFooter } from "./product-footer";

export type PageOptions = {
  owner: symbol;
  screen: Screen;
  search?: PageSearch;
  hideSearch?: boolean;
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
  hideSearch = false,
}: {
  screen: Screen;
  children: ReactNode;
  fullBleed?: boolean;
  search?: PageSearch;
  hideSearch?: boolean;
}) {
  const setOptions = useContext(PageOptionsContext);
  const owner = useRef(Symbol("page"));
  useEffect(() => {
    if (!setOptions) return;
    const id = owner.current;
    setOptions({ owner: id, screen, search, hideSearch });
    return () => {
      setOptions((current) => (current?.owner === id ? null : current));
    };
  }, [screen, search, hideSearch, setOptions]);

  return (
    <div
      data-page={screen}
      data-full-bleed={fullBleed}
      className={cn(
        "product-page",
        !fullBleed && !resolveAdminSection(screen) && "workspace-page",
      )}
    >
      {children}
      {screen !== "assets" &&
        screen !== "subjects" &&
        screen !== "conversations" && <ProductFooter />}
    </div>
  );
}
