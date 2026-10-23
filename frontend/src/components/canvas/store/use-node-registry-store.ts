import { create } from "zustand";

export const useNodeRegistryVersion = create<{ version: number }>(() => ({
  version: 0,
}));
