export const REPOSITORY_ROOT: string;
export const ATTACHMENT_PATHS: readonly string[];
export function isAttachmentPath(sourcePath: string): boolean;
export function readAttachment(
  sourcePath: string,
  repositoryRoot?: string,
): Promise<string | undefined>;
