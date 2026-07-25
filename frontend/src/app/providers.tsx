"use client";

import { useState } from "react";
import { Provider } from "react-redux";

import { type AppStore, makeStore } from "@/store/make-store";

export function Providers({ children }: Readonly<{ children: React.ReactNode }>) {
  const [store] = useState<AppStore>(makeStore);

  return <Provider store={store}>{children}</Provider>;
}
