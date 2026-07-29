import { configureStore } from "@reduxjs/toolkit";

import { appApi } from "@/lib/server-state";

export function makeStore() {
  return configureStore({
    reducer: { [appApi.reducerPath]: appApi.reducer },
    middleware: (getDefaultMiddleware) =>
      getDefaultMiddleware().concat(appApi.middleware),
  });
}

export type AppStore = ReturnType<typeof makeStore>;
