import {
  SCRIPT_MAX_DOCUMENT_BYTES,
  scriptScalarText,
  validRichColor,
  validRichLink,
} from "./rich-document";

const tags = new Set([
  "p",
  "h1",
  "h2",
  "h3",
  "strong",
  "b",
  "em",
  "i",
  "u",
  "s",
  "del",
  "strike",
  "code",
  "pre",
  "blockquote",
  "ul",
  "ol",
  "li",
  "br",
  "hr",
  "span",
  "mark",
  "a",
]);
function unsupported() {
  throw new Error(
    "剪贴板包含无法保留的富文本格式。粘贴已停止；可明确选择仅粘贴纯文本，或取消后保留原格式。",
  );
}
export function assertRichPaste(html: string) {
  if (
    !scriptScalarText.safeParse(html).success ||
    new TextEncoder().encode(html).length > SCRIPT_MAX_DOCUMENT_BYTES
  )
    unsupported();
  const parsed = new DOMParser().parseFromString(html, "text/html");
  if (parsed.head.children.length) unsupported();
  const stack = [...parsed.body.children].map((element) => ({
    element,
    depth: 1,
  }));
  let count = 0;
  while (stack.length) {
    const { element, depth } = stack.pop()!;
    if (++count > 200_000 || depth > 64) unsupported();
    const tag = element.tagName.toLowerCase();
    if (!tags.has(tag)) unsupported();
    for (const attribute of element.attributes) {
      const { name, value } = attribute;
      if (name === "data-pm-slice") continue;
      if (name === "href" && tag === "a" && validRichLink(value)) continue;
      if (
        name === "start" &&
        tag === "ol" &&
        /^\d+$/.test(value) &&
        Number(value) <= 1_000_000
      )
        continue;
      if (
        name === "class" &&
        tag === "code" &&
        element.parentElement?.tagName === "PRE" &&
        /^language-[^\s]+$/.test(value) &&
        new TextEncoder().encode(value.slice(9)).length <= 64
      )
        continue;
      if (name === "data-color" && tag === "mark" && validRichColor(value))
        continue;
      if (name !== "style") unsupported();
      for (const declaration of value.split(";")) {
        if (!declaration.trim()) continue;
        const colon = declaration.indexOf(":");
        const property = declaration.slice(0, colon).trim().toLowerCase();
        const content = declaration.slice(colon + 1).trim();
        if (
          property === "text-align" &&
          ["p", "h1", "h2", "h3"].includes(tag) &&
          ["left", "center", "right", "justify"].includes(content)
        )
          continue;
        if (property === "color" && tag === "span" && validRichColor(content))
          continue;
        if (
          property === "background-color" &&
          tag === "mark" &&
          validRichColor(content)
        )
          continue;
        if (property === "color" && tag === "mark" && content === "inherit")
          continue;
        unsupported();
      }
    }
    if (tag === "a" && !element.hasAttribute("href")) unsupported();
    for (const child of element.children)
      stack.push({ element: child, depth: depth + 1 });
  }
}
