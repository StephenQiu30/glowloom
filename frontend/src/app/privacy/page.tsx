import packageManifest from "../../../public/legal/2026-10-15.1/manifest.json";
import Link from "next/link";
export const metadata = { title: "隐私政策" };
export default function Page() {
  const document = packageManifest.documents[1];
  return (
    <main className="mx-auto max-w-3xl px-5 py-12">
      <Link href="/">返回织光</Link>
      <h1 className="my-6 text-2xl">{document.title}</h1>
      <p>
        协议包 {packageManifest.version} · 本地可审阅版本，运行环境尚未生效。
      </p>
      <p className="my-4 text-xs break-all text-muted-foreground">
        SHA-256：{document.sha256}
      </p>
      <a href="/legal/2026-10-15.1/privacy.md" download>
        保存本文
      </a>
      <pre className="mt-8 font-sans leading-7 break-words whitespace-pre-wrap">
        {document.text}
      </pre>
    </main>
  );
}
