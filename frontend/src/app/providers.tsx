"use client";

import { Provider } from "react-redux";
import { useState } from "react";

import { Toaster } from "@/components/ui/sonner";
import { ThemeProvider } from "@/components/theme-provider";
import { makeStore } from "@/lib/redux-store";

export function AppProviders({ children }: { children: React.ReactNode }) {
  const [store] = useState(makeStore);

  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="system"
      disableTransitionOnChange
      enableSystem
      storageKey="lanverse-theme"
    >
      <Provider store={store}>
        {children}
        <Toaster closeButton containerAriaLabel="操作通知" />
      </Provider>
    </ThemeProvider>
  );
}
