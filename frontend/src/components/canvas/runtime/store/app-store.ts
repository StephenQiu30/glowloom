import {
  configureStore,
  combineReducers,
  createSlice,
  type PayloadAction,
} from "@reduxjs/toolkit";
import type { AssetKind } from "../types/asset";
import { defaultConfig, type AiConfig } from "../types/generation-config";
import type { ThemeMode } from "../lib/theme-preference";
import { DEFAULT_PAGE_SIZE } from "../lib/pagination";
import { accountSlice, accountScope } from "./account-slice";

export type { ThemeMode } from "../lib/theme-preference";
export type OperationScope = "projects" | "assets";
export type WorkspaceOperation = {
  id: string;
  label: string;
  status: "pending" | "success" | "error";
  message: string;
};

const themeSlice = createSlice({
  name: "theme",
  initialState: {
    theme: "light" as ThemeMode,
    preference: "system" as ThemeMode | "system",
  },
  reducers: {
    toggleTheme: (state) => {
      state.theme = state.theme === "dark" ? "light" : "dark";
      state.preference = state.theme;
    },
    setTheme: (state, action: PayloadAction<ThemeMode>) => {
      state.theme = action.payload;
      state.preference = action.payload;
    },
    setSystemTheme: (state, action: PayloadAction<ThemeMode>) => {
      if (state.preference === "system") state.theme = action.payload;
    },
  },
});
const configSlice = createSlice({
  name: "config",
  initialState: { config: defaultConfig },
  reducers: {
    patchConfig: (state, action: PayloadAction<Partial<AiConfig>>) => {
      Object.assign(state.config, action.payload);
    },
  },
});
const workspaceSlice = createSlice({
  name: "workspace",
  initialState: {
    projects: {
      keyword: "",
      sort: "updated" as "updated" | "name",
      pageSize: DEFAULT_PAGE_SIZE,
    },
    assets: {
      keyword: "",
      kind: "all" as AssetKind | "all",
      page: 1,
      pageSize: DEFAULT_PAGE_SIZE,
    },
    operations: {} as Partial<Record<OperationScope, WorkspaceOperation>>,
  },
  reducers: {
    setProjectFilters: (
      state,
      action: PayloadAction<
        Partial<{ keyword: string; sort: "updated" | "name"; pageSize: number }>
      >,
    ) => {
      Object.assign(state.projects, action.payload);
    },
    setAssetFilters: (
      state,
      action: PayloadAction<
        Partial<{
          keyword: string;
          kind: AssetKind | "all";
          page: number;
          pageSize: number;
        }>
      >,
    ) => {
      const patch = action.payload;
      Object.assign(state.assets, patch);
      if (
        patch.keyword !== undefined ||
        patch.kind !== undefined ||
        patch.pageSize !== undefined
      )
        state.assets.page = 1;
    },
    startOperation: (
      state,
      action: PayloadAction<{
        scope: OperationScope;
        id: string;
        label: string;
      }>,
    ) => {
      const { scope, id, label } = action.payload;
      if (state.operations[scope]?.status === "pending") return;
      state.operations[scope] = { id, label, status: "pending", message: "" };
    },
    finishOperation: (
      state,
      action: PayloadAction<{
        scope: OperationScope;
        id: string;
        status: "success" | "error";
        message: string;
      }>,
    ) => {
      const { scope, id, status, message } = action.payload;
      const operation = state.operations[scope];
      if (operation?.id !== id || operation.status !== "pending") return;
      operation.status = status;
      operation.message = message;
    },
  },
});
export const themeActions = themeSlice.actions;
export const configActions = configSlice.actions;
export const workspaceActions = workspaceSlice.actions;

const reducers = combineReducers({
  account: accountSlice.reducer,
  theme: themeSlice.reducer,
  config: configSlice.reducer,
  workspace: workspaceSlice.reducer,
});
const reducer: typeof reducers = (state, action) => {
  const next = reducers(state, action);
  if (
    state &&
    accountScope(state.account?.account ?? null) !==
      accountScope(next.account.account)
  ) {
    return {
      ...next,
      config: configSlice.getInitialState(),
      workspace: workspaceSlice.getInitialState(),
    };
  }
  return next;
};

/** Each Provider owns its store; server renders never share mutable UI state. */
export function makeAppStore(initialTheme?: ThemeMode) {
  return configureStore({
    reducer,
    preloadedState: initialTheme
      ? { theme: { theme: initialTheme, preference: initialTheme } }
      : undefined,
  });
}
export type AppStore = ReturnType<typeof makeAppStore>;
export type AppState = ReturnType<AppStore["getState"]>;
export type AppDispatch = AppStore["dispatch"];
