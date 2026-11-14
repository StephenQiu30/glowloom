/** A frozen content version always queries its own index, including its visibility scope. */
export function searchIndexName(contentDigest: string): string {
  if (!/^[a-f0-9]{64}$/.test(contentDigest)) {
    throw new Error('Invalid search content digest');
  }
  return `workspace_${contentDigest}`;
}
