import { create } from "zustand";

export const CANVAS_SIDE_PANEL_MOTION_MS = 500;
export const CANVAS_SIDE_PANEL_MIN_WIDTH = 280;
export const CANVAS_SIDE_PANEL_MAX_WIDTH = 480;
export const CANVAS_SIDE_PANEL_DEFAULT_WIDTH = 320;

type CanvasSidePanelStore = {
  tab: "canvas" | "assets";
  setTab: (tab: "canvas" | "assets") => void;
  width: number;
  panelOpen: boolean;
  panelMounted: boolean;
  panelClosing: boolean;
  setWidth: (width: number) => void;
  openPanel: () => void;
  closePanel: () => void;
  togglePanel: () => void;
};

export const useCanvasSidePanelStore = create<CanvasSidePanelStore>(
  (set, get) => ({
    tab: "canvas",
    setTab: (tab) => set({ tab }),
    width: CANVAS_SIDE_PANEL_DEFAULT_WIDTH,
    panelOpen: false,
    panelMounted: false,
    panelClosing: false,
    setWidth: (width) => set({ width }),
    openPanel: () => {
      set({ panelOpen: true, panelMounted: true, panelClosing: false });
    },
    closePanel: () => {
      if (!get().panelMounted || get().panelClosing) return;
      set({ panelOpen: false, panelClosing: true });
      setTimeout(() => {
        if (get().panelClosing)
          set({ panelMounted: false, panelClosing: false });
      }, CANVAS_SIDE_PANEL_MOTION_MS);
    },
    togglePanel: () =>
      get().panelOpen ? get().closePanel() : get().openPanel(),
  }),
);
