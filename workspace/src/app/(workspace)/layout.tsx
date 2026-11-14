import type { ReactNode } from 'react';
import { getSnapshot, categoryNames } from '@/content/runtime';
import { Navigation } from '@/components/navigation';

export default async function WorkspaceLayout({
  children,
}: {
  readonly children: ReactNode;
}) {
  const snapshot = await getSnapshot();
  return (
    <Navigation
      items={snapshot.documents.map(({ id, title, category }) => ({
        id,
        title,
        category,
      }))}
      categories={categoryNames}
      online={process.env.NEXT_PUBLIC_WORKSPACE_MODE === 'online'}
      preview={snapshot.preview}
    >
      {children}
    </Navigation>
  );
}
