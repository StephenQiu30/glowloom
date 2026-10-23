export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";
const palette = () => ({
  canvas: {
    background: "var(--canvas-background)",
    dot: "var(--canvas-dot)",
    line: "var(--canvas-line)",
    selectionStroke: "var(--ring)",
    selectionFill: "var(--canvas-selection-fill)",
  },
  node: {
    label: "var(--brand-body)",
    fill: "var(--muted)",
    panel: "var(--card)",
    stroke: "var(--border)",
    activeStroke: "var(--ring)",
    placeholder: "var(--brand-muted)",
    text: "var(--foreground)",
    muted: "var(--muted-foreground)",
    faint: "var(--canvas-faint)",
  },
  toolbar: {
    panel: "var(--canvas-toolbar)",
    border: "var(--border)",
    item: "var(--brand-body)",
    itemHover: "var(--canvas-toolbar-hover)",
    activeBg: "var(--accent)",
    activeText: "var(--foreground)",
  },
});
export const canvasThemes = { light: palette(), dark: palette() };
export type CanvasTheme = (typeof canvasThemes)[CanvasColorTheme];
