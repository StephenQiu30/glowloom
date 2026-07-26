import { configureStore } from "@reduxjs/toolkit";

import { backendApi } from "@/store/backend-api";
import { uiReducer } from "@/store/slices/ui-slice";

export function makeStore() {
  return configureStore({
    reducer: {
      [backendApi.reducerPath]: backendApi.reducer,
      ui: uiReducer,
    },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(backendApi.middleware),
  });
}

export type AppStore = ReturnType<typeof makeStore>;
export type RootState = ReturnType<AppStore["getState"]>;
export type AppDispatch = AppStore["dispatch"];
