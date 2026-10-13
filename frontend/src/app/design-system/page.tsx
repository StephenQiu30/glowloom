import Link from "next/link";
export default function Page() {
  return (
    <main className="session-state">
      <div>
        <h1>织光 R6 设计</h1>
        <p>当前设计稿、完整流程与异常状态统一在 Figma 中维护。</p>
        <a
          href="https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/?node-id=365-17335"
          className="underline"
        >
          查看设计稿
        </a>
        <br />
        <Link href="/">返回创作</Link>
      </div>
    </main>
  );
}
