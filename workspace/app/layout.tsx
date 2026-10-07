import type { Metadata } from "next";
import type { PageMapItem } from "nextra";
import { Head, Search } from "nextra/components";
import { getPageMap } from "nextra/page-map";
import { Footer, Layout, Navbar } from "nextra-theme-docs";
import type { ReactNode } from "react";
import "nextra-theme-docs/style.css";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Lanverse 知识库", template: "%s · Lanverse 知识库" },
  description: "Lanverse 的产品、需求、设计、计划与验证文档。",
};

// Nextra 4.6.1 generates raw routes, while Next.js usePathname keeps URL encoding.
function encodePageMapRoutes(items: PageMapItem[]): PageMapItem[] {
  return items.map((item) => {
    if ("children" in item) {
      return {
        ...item,
        route: encodeURI(item.route),
        children: encodePageMapRoutes(item.children),
      };
    }
    return "route" in item ? { ...item, route: encodeURI(item.route) } : item;
  });
}

export default async function RootLayout({
  children,
}: {
  children: ReactNode;
}) {
  const pageMap = encodePageMapRoutes(await getPageMap());

  return (
    <html lang="zh-CN" dir="ltr" suppressHydrationWarning>
      <Head faviconGlyph="L" color={{ lightness: { light: 35, dark: 55 } }} />
      <body>
        <Layout
          navbar={<Navbar logo={<strong>Lanverse 知识库</strong>} />}
          footer={<Footer>Lanverse · 工作空间文档</Footer>}
          pageMap={pageMap}
          search={
            <Search
              placeholder="搜索文档…"
              aria-label="搜索文档"
              loading="正在查找…"
              emptyResult="没有找到匹配的文档。"
              errorText="搜索索引尚未生成，请先运行构建。"
            />
          }
          editLink={null}
          copyPageButton={false}
          feedback={{ content: null }}
          sidebar={{ autoCollapse: true, defaultMenuCollapseLevel: 1 }}
          themeSwitch={{ light: "浅色", dark: "深色", system: "跟随系统" }}
          toc={{ title: "本页目录", backToTop: "回到顶部" }}
        >
          {children}
        </Layout>
      </body>
    </html>
  );
}
