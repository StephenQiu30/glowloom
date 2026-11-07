import { subscribeContextChange } from "@/lib/request";

export const redrawMarkets = [
  ["原片语言", "沿用原片"],
  ["中国大陆", "中国大陆"],
  ["海外", "海外"],
] as const;

export type StudioInput =
  | { kind: "create"; prompt: string }
  | { kind: "upload"; text: string; file?: File }
  | { kind: "redraw"; file?: File; style: string; market: string };

let pending:
  { owner: string; projectID: string; input: StudioInput } | undefined;

subscribeContextChange(() => {
  pending = undefined;
});

export function saveStudioInput(
  owner: string,
  projectID: string,
  input: StudioInput,
) {
  pending = { owner, projectID, input };
}

export function readStudioInput(owner: string, projectID: string) {
  if (pending?.owner === owner && pending.projectID === projectID) {
    return pending.input;
  }
}

export function clearStudioInput(input: StudioInput) {
  if (pending?.input === input) pending = undefined;
}
