export function parseStyleAnalysisReply(text: string): {
  name: string;
  description: string;
} {
  const match =
    /^风格名称：[ \t]*([^\n]+)\n(?:[ \t]*\n)+风格描述：[ \t]*([\s\S]*)$/u.exec(
      text.replace(/\r\n?/g, "\n").trim(),
    );
  if (!match)
    throw new Error(
      "分析回复格式不正确，请按“风格名称：… / 风格描述：…”重新返回。",
    );
  const name = match[1]!.trim(),
    description = match[2]!.trim();
  if (!name || Array.from(name).length > 20)
    throw new Error("分析名称必须为 1 至 20 字，请修改后再采用。");
  if (!description || Array.from(description).length > 2000)
    throw new Error("分析描述必须为 1 至 2000 字，请修改后再采用。");
  return { name, description };
}
