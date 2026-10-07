import type { Root } from "mdast";
export interface SourceLinkOptions {
  repositoryRoot?: string;
  contentRoot?: string;
}
export function resolveSourceLink(
  sourceFile: string,
  href: string,
  options?: SourceLinkOptions,
): string;
export default function remarkSourceLinks(
  options?: SourceLinkOptions,
): (tree: Root, file: { path?: string }) => void;
