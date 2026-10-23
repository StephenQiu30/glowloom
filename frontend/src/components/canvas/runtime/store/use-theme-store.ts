"use client";

import { useMemo } from "react";
import { themeActions, type ThemeMode } from "./app-store";
import { useAppDispatch, useAppSelector } from "./hooks";
export type { ThemeMode } from "./app-store";
type ThemeStore = {
  theme: ThemeMode;
  setTheme: (theme: ThemeMode) => void;
  toggleTheme: () => void;
};
export function useThemeStore<T>(selector: (state: ThemeStore) => T) {
  const dispatch = useAppDispatch();
  const actions = useMemo(
    () => ({
      toggleTheme: () => dispatch(themeActions.toggleTheme()),
      setTheme: (theme: ThemeMode) => {
        dispatch(themeActions.setTheme(theme));
      },
    }),
    [dispatch],
  );
  return useAppSelector((state) => selector({ ...state.theme, ...actions }));
}
