import { Node, mergeAttributes, type Attributes } from "@tiptap/core";
import Blockquote from "@tiptap/extension-blockquote";
import Code from "@tiptap/extension-code";
import Highlight from "@tiptap/extension-highlight";
import { OrderedList } from "@tiptap/extension-list";
import TextAlign from "@tiptap/extension-text-align";
import { Color, TextStyle } from "@tiptap/extension-text-style";
import StarterKit from "@tiptap/starter-kit";
import { validRichLink } from "./rich-document";

const ScriptDocument = Node.create({
  name: "doc",
  topNode: true,
  content: "block*",
});
const ScriptOrderedList = OrderedList.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      start: {
        default: null,
        parseHTML: (element: HTMLElement) =>
          element.hasAttribute("start")
            ? Number(element.getAttribute("start"))
            : null,
        renderHTML: (attrs: Record<string, unknown>) =>
          attrs.start === null ? {} : { start: attrs.start },
      },
    };
  },
  renderHTML({ HTMLAttributes }) {
    return [
      "ol",
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes),
      0,
    ];
  },
});
const ScriptHighlight = Highlight.extend({
  addAttributes() {
    const attributes: Attributes = this.parent?.() ?? {};
    return {
      ...attributes,
      color: { ...attributes.color, default: "#fef08a" },
    };
  },
});

export function scriptEditorExtensions() {
  return [
    StarterKit.configure({
      document: false,
      blockquote: false,
      code: false,
      orderedList: false,
      trailingNode: false,
      heading: { levels: [1, 2, 3] },
      link: {
        openOnClick: false,
        autolink: true,
        HTMLAttributes: { target: null, rel: null, class: null },
        isAllowedUri: validRichLink,
      },
    }),
    ScriptDocument,
    Blockquote.extend({ content: "block*" }),
    Code.extend({ excludes: "" }),
    ScriptOrderedList,
    TextAlign.configure({ types: ["heading", "paragraph"] }),
    TextStyle,
    Color,
    ScriptHighlight.configure({ multicolor: true }),
  ];
}
