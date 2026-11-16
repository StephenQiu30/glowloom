export type CanvasBackgroundMode = "lines" | "dots" | "blank";
// Let the host's existing light/dark theme own colors, without the upstream app store.
export const theme = {
  canvas: {
    background: "var(--background)",
    dot: "var(--muted-foreground)",
    line: "var(--border)",
  },
  node: { activeStroke: "var(--primary)", muted: "var(--muted-foreground)" },
  toolbar: { panel: "var(--popover)", border: "var(--border)" },
};
