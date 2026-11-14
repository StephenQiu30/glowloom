import { Operations } from "@/online/components/operations";
export const metadata = { title: "在线协作台" };
export default function OperationsPage() {
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>在线协作台</h1>
          <p>审阅提案、固定任务上下文，按阶段核对真实交付。</p>
        </div>
      </div>
      <Operations />
    </div>
  );
}
