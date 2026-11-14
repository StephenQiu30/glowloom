import { Handoff } from "@/components/handoff";
import { getSnapshot } from "@/content/runtime";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
export const metadata = { title: "任务与交接" };
export default async function TasksPage() {
  const snapshot = await getSnapshot();
  const online = process.env.NEXT_PUBLIC_WORKSPACE_MODE === "online";
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>任务与交接</h1>
          <p>以固定版本上下文进入任务，保留每次执行与接受的依据。</p>
        </div>
        {online && (
          <Button asChild variant="outline">
            <Link href="/operations/">打开协作台</Link>
          </Button>
        )}
      </div>
      <Alert>
        <AlertTitle>
          {online ? "进入在线协作台管理任务" : "当前为静态阅读版"}
        </AlertTitle>
        <AlertDescription>
          {online
            ? "创建任务、审阅文档提案、核对运行状态和交接产物。执行前检查主机及授权。"
            : "可阅读工作流和 Skill，实际运行、文档编辑及恢复需进入独立在线版。"}
        </AlertDescription>
      </Alert>
      <Handoff
        documents={snapshot.documents.map(
          ({ id, title, revision, path, sourcePath }) => ({
            id,
            title,
            revision,
            path,
            sourcePath,
          }),
        )}
        repoCommit={snapshot.repoCommit}
        preview={snapshot.preview}
      />
      <div className="actions" style={{ marginTop: 24 }}>
        <Button asChild variant="outline">
          <Link href="/workflows/">阅读工作流</Link>
        </Button>
        <Button asChild variant="ghost">
          <Link href="/engineering/">定位工程</Link>
        </Button>
      </div>
    </div>
  );
}
