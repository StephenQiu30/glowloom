import { loadSnapshot, validateLinks } from './catalog';
/** Validates the selected knowledge scope and reports its frozen identity. */
export async function validateContent(
  workspaceRoot: string,
  scope: 'public' | 'internal' = 'internal',
) {
  const snapshot = await loadSnapshot({ workspaceRoot, scope });
  const errors = await validateLinks(snapshot);
  if (errors.length) {
    throw new Error(JSON.stringify(errors, null, 2));
  }
  return {
    documents: snapshot.documents.length,
    contentDigest: snapshot.contentDigest,
    scope,
  };
}
