import type { Metadata } from "next";
import { Geist } from "next/font/google";

import { cn } from "@/lib/class-names";

import { AppProviders } from "./providers";

import "./globals.css";

const geist = Geist({ subsets: ["latin"], variable: "--font-geist" });

export const metadata: Metadata = {
  title: "Lanverse · AI 短剧制作平台",
  description: "从剧本、分镜、生成到审核交付的 AI 短剧制作工作台",
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
