import type { Metadata } from "next";
import { Geist } from "next/font/google";

import { Providers } from "@/app/providers";
import { cn } from "@/lib/utils";

import "./globals.css";

const geist = Geist({
  subsets: ["latin"],
  variable: "--font-geist-sans",
});

export const metadata: Metadata = {
  title: "Lanverse · AI 短剧制作",
  description: "从故事、剧本和分镜到媒体生成与成片交付的制作工作台。",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      className={cn("font-sans", geist.variable)}
      lang="zh-CN"
      suppressHydrationWarning
    >
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
