import type { Metadata } from "next";
import { Geist } from "next/font/google";

import { cn } from "@/lib/class-names";

import { AppProviders } from "./providers";

import "./globals.css";

const geist = Geist({ subsets: ["latin"], variable: "--font-geist" });

export const metadata: Metadata = {
  title: "Lanverse · AI 漫剧制作平台",
  description: "从灵感、资产、分镜到生成交付的一站式 AI 漫剧制作工作台",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" className={cn("font-sans", geist.variable)}>
      <body>
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
