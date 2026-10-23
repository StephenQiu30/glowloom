"use client";

import { useRef } from "react";

/** 相同草稿重试沿用命令标识，凭据及命令内容只在组件内存中。 */
export function useAccountCommand() {
  const commands = useRef(new Map<string, { payload: string; id: string }>());
  return <T extends object>(
    operation: string,
    data: T,
  ): T & { commandId: string } => {
    const payload = JSON.stringify(data);
    let command = commands.current.get(operation);
    if (!command || command.payload !== payload) {
      command = { payload, id: crypto.randomUUID() };
      commands.current.set(operation, command);
    }
    return { ...data, commandId: command.id };
  };
}
