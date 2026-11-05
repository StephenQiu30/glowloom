import type { TranscriptContext } from "@earendil-works/pi-ai";
export function businessTranscript(
  context: Pick<TranscriptContext, "messages">,
) {
  const tools = new Map<
    string,
    { name: string; description: string; parameters: unknown }
  >();
  const messages: Record<string, unknown>[] = [];
  const system: string[] = [];
  const sections = new Map<string, string>();
  for (const msg of context.messages) {
    if (msg.role === "system") {
      if (msg.content)
        system.push(
          typeof msg.content === "string"
            ? msg.content
            : msg.content.map((c) => c.text).join("\n"),
        );
      for (const t of msg.toolsAdded ?? [])
        tools.set(t.name, {
          name: t.name,
          description: t.description,
          parameters: t.parameters,
        });
      for (const name of msg.toolsRemoved ?? []) tools.delete(name.name);
      for (const [key, value] of Object.entries(msg.sections ?? {})) {
        if (value === null) sections.delete(key);
        else sections.set(key, value);
      }
    } else if (msg.role === "assistant") {
      const text = msg.content
        .filter((c) => c.type === "text")
        .map((c) => c.text)
        .join("\n");
      const calls = msg.content
        .filter((c) => c.type === "toolCall")
        .map((c) => ({
          id: c.id,
          type: "function",
          function: { name: c.name, arguments: JSON.stringify(c.arguments) },
        }));
      messages.push({
        role: "assistant",
        content: text,
        ...(calls.length ? { tool_calls: calls } : {}),
      });
    } else if (msg.role === "toolResult") {
      messages.push({
        role: "tool",
        tool_call_id: msg.toolCallId,
        content: msg.content
          .filter((c) => c.type === "text")
          .map((c) => c.text)
          .join("\n"),
      });
      const images = msg.content
        .filter((c) => c.type === "image")
        .map((c) => ({
          type: "image_url",
          image_url: { url: `data:${c.mimeType};base64,${c.data}` },
        }));
      if (images.length)
        messages.push({
          role: "user",
          content: [
            {
              type: "text",
              text: `Authorized media returned by tool call ${msg.toolCallId}; inspect the bytes before describing them.`,
            },
            ...images,
          ],
        });
    } else if (msg.role === "user") {
      const content =
        typeof msg.content === "string"
          ? msg.content
          : msg.content.map((c) =>
              c.type === "text"
                ? { type: "text", text: c.text }
                : {
                    type: "image_url",
                    image_url: { url: `data:${c.mimeType};base64,${c.data}` },
                  },
            );
      messages.push({ role: "user", content });
    }
  }
  const prompt = [...system, ...sections.values()];
  if (prompt.length)
    messages.unshift({ role: "system", content: prompt.join("\n\n") });
  return { messages, tools: [...tools.values()] };
}
