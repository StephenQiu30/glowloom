"use client";

import {
  createContext,
  useContext,
  useLayoutEffect,
  useRef,
  type Dispatch,
  type SetStateAction,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

export const PageToolbarContext = createContext<HTMLElement | null | undefined>(
  undefined,
);

export type PageActionsRegistration = {
  owner: symbol;
  presentation: "adjacent" | "replace-base";
};
export const PageActionsContext = createContext<
  | {
      target: HTMLElement | null;
      setPresentation: Dispatch<SetStateAction<PageActionsRegistration | null>>;
    }
  | undefined
>(undefined);

/** Keep page state and context with the page while placing its controls in the shell. */
export function PageToolbar({
  inHeader,
  children,
  slot = "navigation",
  presentation = "adjacent",
}: {
  inHeader: boolean;
  children: ReactNode;
  slot?: "navigation" | "actions";
  presentation?: PageActionsRegistration["presentation"];
}) {
  const navigation = useContext(PageToolbarContext);
  const actions = useContext(PageActionsContext);
  const owner = useRef(Symbol("page-actions"));
  const target = slot === "actions" ? actions?.target : navigation;
  const setPresentation = actions?.setPresentation;
  useLayoutEffect(() => {
    if (!inHeader || slot !== "actions" || !target || !setPresentation) return;
    const id = owner.current;
    setPresentation({ owner: id, presentation });
    return () =>
      setPresentation((current) => (current?.owner === id ? null : current));
  }, [inHeader, slot, target, setPresentation, presentation]);
  if (!inHeader || target === undefined) return children;
  return target ? createPortal(children, target) : null;
}
