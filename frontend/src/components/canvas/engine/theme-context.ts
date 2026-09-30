import { useTheme } from "next-themes";
export function useActiveTheme(): "light" | "dark" {
  return useTheme().resolvedTheme === "dark" ? "dark" : "light";
}
