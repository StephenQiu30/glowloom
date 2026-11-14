import type { ReactNode } from 'react';
import { getSnapshot, getDocumentSource } from '@/content/runtime';
import { Navigation } from '@/components/navigation';

export default async function WorkspaceLayout({
  children,
}: {
  readonly children: ReactNode;
}) {
  const [source, snapshot] = await Promise.all([
    getDocumentSource(),
    getSnapshot(),
  ]);
  return (
    <Navigation
      tree={source.pageTree}
      online={process.env.NEXT_PUBLIC_WORKSPACE_MODE === 'online'}
      preview={snapshot.preview}
    >
      {children}
    </Navigation>
  );
}
