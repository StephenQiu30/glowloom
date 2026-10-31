import type { Metadata } from "next";
import { cookies } from "next/headers";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import { Providers } from "./_components/providers";
import "./globals.css";

export const metadata: Metadata = {
  applicationName: "织光 Glowloom",
  title: { default: "织光 · 创作工作台", template: "%s" },
  description: "AI 短剧制作平台",
  icons: {
    icon: [
      {
        url: "/brand/icons/favicon.ico",
        sizes: "16x16 32x32 48x48",
        type: "image/x-icon",
      },
      { url: "/brand/logo.svg", sizes: "any", type: "image/svg+xml" },
    ],
    apple: {
      url: "/brand/icons/apple-touch-icon.png",
      sizes: "180x180",
      type: "image/png",
    },
  },
};

export default async function RootLayout({ children }: LayoutProps<"/">) {
  const sidebarOpen = (await cookies()).get("sidebar_state")?.value !== "false";
  return (
    <html
      lang="zh-CN"
      suppressHydrationWarning
      className={`${GeistSans.variable} ${GeistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <Providers initialSidebarOpen={sidebarOpen}>{children}</Providers>
      </body>
    </html>
  );
}
