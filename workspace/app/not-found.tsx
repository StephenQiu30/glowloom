import { useMDXComponents as getMDXComponents } from "../mdx-components";

const { h1: Heading, p: Paragraph, a: Anchor } = getMDXComponents();

export default function NotFound() {
  return (
    <main className="not-found-page">
      <Heading>找不到这篇文档</Heading>
      <Paragraph>文档可能已改名或移动。</Paragraph>
      <Paragraph>
        <Anchor href="/">返回知识库</Anchor>
      </Paragraph>
    </main>
  );
}
