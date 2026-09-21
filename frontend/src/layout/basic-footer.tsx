import { LayoutContainer } from "@/layout/layout-container";

export function BasicFooter({ compact = false }: { compact?: boolean }) {
  return (
    <footer className="basic-layout__footer bg-background" aria-label="Lanverse 页脚">
      <LayoutContainer className="flex min-h-12 items-center justify-between gap-4 py-4 text-xs text-muted-foreground">
        <span>© 2026 Lanverse · 安全创作环境</span>
        {!compact && <span className="hidden sm:inline">可追溯的创作工作区</span>}
      </LayoutContainer>
    </footer>
  );
}
