"use client";

import { ThemeProvider } from "next-themes";
import { GeistSans } from "geist/font/sans";
import { PageFailure } from "@/components/layout/application-state";
import "./globals.css";

export default function GlobalError({ retry }: { retry: () => void }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning className={GeistSans.variable}>
      <body>
        <title>页面加载失败 · 织光</title>
        <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
          <PageFailure retry={retry} standalone />
        </ThemeProvider>
      </body>
    </html>
  );
}
