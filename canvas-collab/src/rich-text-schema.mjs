import { getSchema, Node } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Mention from "@tiptap/extension-mention";
import TextAlign from "@tiptap/extension-text-align";

const mention = Mention.extend({
  addAttributes() {
    return { ...this.parent?.(), token: { default: null } };
  },
});
// Glowloom stores reference identity separately from transient preview URLs and labels.
const reference = Node.create({
  name: "reference",
  group: "inline",
  inline: true,
  atom: true,
  addAttributes: () => ({ referenceId: { default: null } }),
  renderHTML: ({ node }) => [
    "span",
    { "data-reference-id": node.attrs.referenceId },
  ],
});
export const richTextSchema = getSchema([
  StarterKit.configure({ undoRedo: false }),
  TextAlign.configure({
    types: ["paragraph", "heading"],
    alignments: ["left", "center", "right"],
  }),
  mention,
  reference,
]);
const attrs = {
  paragraph: ["textAlign"],
  heading: ["level", "textAlign"],
  orderedList: ["start"],
  codeBlock: ["language"],
  mention: ["id", "label", "token"],
  reference: ["referenceId"],
  link: ["href", "target", "rel", "class"],
};

/** Remove editor-only attributes, never silently discard unknown business data. */
export function persistentRichText(value) {
  const node = structuredClone(value);
  const walk = (item, depth = 0) => {
    if (depth > 32 || !item || typeof item !== "object")
      throw new Error("INVALID_RICH_TEXT");
    if (
      item.type === "reference" &&
      (typeof item.attrs?.referenceId !== "string" ||
        !item.attrs.referenceId.length ||
        item.attrs.referenceId.length > 128)
    )
      throw new Error("INVALID_REFERENCE_IDENTITY");
    if (item.attrs) {
      if (item.type === "mention") delete item.attrs.mentionSuggestionChar;
      for (const field of Object.keys(item.attrs)) {
        if (
          field === "textAlign" &&
          ["paragraph", "heading"].includes(item.type)
        ) {
          // Tiptap adds a null default. Omit it so opening legacy documents
          // cannot alter their persisted representation or accepted graph hash.
          if (item.attrs.textAlign === null) delete item.attrs.textAlign;
          else if (!["left", "center", "right"].includes(item.attrs.textAlign))
            throw new Error("INVALID_TEXT_ALIGNMENT");
          continue;
        }
        if (
          (item.type === "orderedList" && field === "type") ||
          (item.type === "link" && field === "title")
        ) {
          if (item.attrs[field] !== null)
            throw new Error("UNREGISTERED_TEXT_ATTRIBUTE");
          delete item.attrs[field];
        } else if (!(attrs[item.type] ?? []).includes(field))
          throw new Error("UNREGISTERED_TEXT_ATTRIBUTE");
      }
      if (!Object.keys(item.attrs).length) delete item.attrs;
    }
    if (item.content) item.content.forEach((child) => walk(child, depth + 1));
    if (item.marks) item.marks.forEach((mark) => walk(mark, depth + 1));
  };
  walk(node);
  return node;
}

export function normalizeRichText(value) {
  const input = persistentRichText(value);
  const node = richTextSchema.nodeFromJSON(input);
  node.check();
  return persistentRichText(node.toJSON());
}
