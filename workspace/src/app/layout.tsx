import type { Metadata } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import { Theme } from "@/components/theme";
import "./globals.css";
export const metadata: Metadata = {
  title: {
    default: "知识库 · 织光 Workspace",
    template: "%s · 织光 Workspace",
  },
  description: "织光项目知识、Skill 与研发工作流。",
};
export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body className={`${GeistSans.variable} ${GeistMono.variable}`}>
        <Theme>
          <a href="#main" className="skip-link">
            跳至正文
          </a>
          {children}
        </Theme>
      </body>
    </html>
  );
}
