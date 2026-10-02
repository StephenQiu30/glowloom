import type { SourceExtractionWarning } from "./source-model";

const labels: Record<SourceExtractionWarning["code"], string> = {
  unsupported_document_element: "不支持的文档元素已略去",
  table_layout_flattened: "表格布局已展开为正文",
  document_layout_omitted: "文档版式未进入正文",
  unsupported_heading_style: "标题样式无法完整保留",
  paragraph_style_omitted: "段落样式未进入正文",
  unsupported_alignment: "段落对齐方式无法保留",
  paragraph_layout_omitted: "段落版式未进入正文",
  unsupported_paragraph_content: "不支持的段落内容已略去",
  unsupported_run_content: "不支持的文字内容已略去",
  embedded_media_omitted: "内嵌媒体未进入正文",
  unsupported_hyperlink: "超链接无法完整保留",
  document_revision_omitted: "文档修订标记未进入正文",
  underline_style_omitted: "下划线样式未进入正文",
  unsupported_text_color: "文字颜色无法保留",
  unsupported_highlight_color: "高亮颜色无法保留",
  run_format_omitted: "部分文字格式未进入正文",
  duplicate_run_format: "重复文字格式已归一化",
  unsupported_numbering_format: "编号格式无法完整保留",
  numbering_missing_parent: "编号缺少上级定义",
  numbered_heading_as_paragraph: "带编号标题已保留为段落",
};

export function SourceExtractionWarnings({
  warnings,
}: {
  warnings?: SourceExtractionWarning[];
}) {
  if (!warnings?.length) return null;
  return (
    <section
      className="space-y-2 rounded-lg border p-3 text-sm"
      aria-label="原件提取警告"
    >
      <h3 className="font-semibold">原件提取警告</h3>
      <p>
        以下事实属于此原件快照。请对照原件检查正文；手工保存的新快照仍可沿历史读取原件和警告。
      </p>
      <ul className="list-inside list-disc space-y-1">
        {warnings.map((warning) => (
          <li
            key={warning.code}
          >{`${labels[warning.code]} · ${warning.count.toLocaleString("zh-CN")} 处${warning.paragraph === undefined ? "" : ` · 原件第 ${warning.paragraph + 1} 段`}`}</li>
        ))}
      </ul>
    </section>
  );
}
