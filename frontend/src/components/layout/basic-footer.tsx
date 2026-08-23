import { LayoutContainer } from "./layout-container";

export function BasicFooter() {
  return (
    <footer className="basic-layout__footer bg-background" aria-label="Lanverse 页脚">
      <LayoutContainer className="flex h-full items-center justify-between gap-4 text-xs text-muted-foreground">
        <span>© 2026 Lanverse · 安全创作环境</span>
        <span className="hidden sm:inline">可追溯的创作工作区</span>
      </LayoutContainer>
    </footer>
  );
}
