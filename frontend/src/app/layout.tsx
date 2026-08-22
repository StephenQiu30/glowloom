import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Lanverse 剧本事实工作台",
  description: "从整本剧本提取剧集、人物、资产和场景事实。",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
