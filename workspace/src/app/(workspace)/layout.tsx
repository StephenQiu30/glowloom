import { getSnapshot, categoryNames } from "@/content/runtime";
import { Navigation } from "@/components/navigation";
import { ThemeToggle } from "@/components/theme";
export default async function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const snapshot = await getSnapshot();
  return (
    <>
      <Navigation
        items={snapshot.documents.map(({ id, title, category }) => ({
          id,
          title,
          category,
        }))}
        categories={categoryNames}
      />
      <div className="workspace-main">
        <header className="topbar" data-pagefind-ignore>
          <span className="topbar-label">
            Glowloom <span>/</span> 项目工作空间
          </span>
          <div className="topbar-tools">
            <span className="version-label">
              {snapshot.preview ? "本地预览" : snapshot.repoCommit.slice(0, 8)}
              <span className="version-dot" />
              {process.env.NEXT_PUBLIC_WORKSPACE_MODE === "online"
                ? "在线协作"
                : "静态阅读"}
            </span>
            <ThemeToggle />
          </div>
        </header>
        <main id="main" tabIndex={-1}>
          {children}
        </main>
        <footer className="site-footer" data-pagefind-ignore>
          <span>知识与工程共用同一版本</span>
          <code>{snapshot.contentDigest.slice(0, 12)}</code>
        </footer>
      </div>
    </>
  );
}
