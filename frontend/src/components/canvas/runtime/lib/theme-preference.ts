export type ThemeMode = "light" | "dark";
export const THEME_COOKIE = "glowloom_canvas_theme_v1";

export function parseThemePreference(
  value: string | undefined,
): ThemeMode | undefined {
  return value === "light" || value === "dark" ? value : undefined;
}
