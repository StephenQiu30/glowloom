import { createHash } from "node:crypto";
import GithubSlugger from "github-slugger";
import { toString } from "mdast-util-to-string";
import type { Root } from "mdast";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import remarkRehype from "remark-rehype";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import rehypeStringify from "rehype-stringify";
import { visit } from "unist-util-visit";
import type { ParsedDocument } from "./types";

export const digest = (value: string | Uint8Array) =>
  createHash("sha256").update(value).digest("hex");

export async function parseMarkdown(
  source: string,
  resolveHref: (href: string) => string = (href) => href,
): Promise<ParsedDocument> {
  const result: ParsedDocument = {
    title: "",
    html: "",
    headings: [],
    blocks: [],
    links: [],
    diagrams: [],
  };
  const processor = unified().use(remarkParse).use(remarkGfm);
  const tree = processor.parse(source) as Root;
  const slugger = new GithubSlugger();
  const occurrences = new Map<string, number>();
  let heading = "";
  visit(tree, (node) => {
    if (node.type === "heading") {
      const text = toString(node);
      const id = slugger.slug(text);
      heading = id;
      node.data = { ...node.data, hProperties: { id } };
      result.headings.push({ id, text, depth: node.depth });
      if (!result.title) result.title = text;
    }
    if (node.type === "paragraph") {
      const text = toString(node).trim();
      if (!text) return;
      const fingerprint = digest(text).slice(0, 14);
      const occurrence = occurrences.get(fingerprint) ?? 0;
      occurrences.set(fingerprint, occurrence + 1);
      const id = `block-${fingerprint}-${occurrence}`;
      node.data = { ...node.data, hProperties: { id, "data-block-id": id } };
      result.blocks.push({
        id,
        text,
        heading,
        start: node.position?.start.offset ?? 0,
        end: node.position?.end.offset ?? 0,
      });
    }
    if (
      node.type === "link" ||
      node.type === "image" ||
      node.type === "definition"
    ) {
      result.links.push(node.url);
      node.url = resolveHref(node.url);
    }
    if (node.type === "code" && node.lang === "mermaid") {
      const id = digest(node.value).slice(0, 20);
      result.diagrams.push({ id, source: node.value });
      Object.assign(node, {
        type: "html",
        value: `<figure data-diagram="${id}"></figure>`,
      });
    }
  });
  const renderer = unified()
    .use(remarkRehype, { allowDangerousHtml: true })
    .use(rehypeRaw)
    .use(rehypeSanitize, {
      ...defaultSchema,
      clobberPrefix: "",
      tagNames: [...(defaultSchema.tagNames ?? []), "figure"],
      attributes: {
        ...defaultSchema.attributes,
        "*": [...(defaultSchema.attributes?.["*"] ?? []), "id"],
        p: ["id", "dataBlockId"],
        figure: ["dataDiagram"],
      },
    })
    .use(rehypeStringify);
  result.html = renderer.stringify(await renderer.run(tree));
  return result;
}
