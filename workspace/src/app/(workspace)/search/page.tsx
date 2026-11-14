import { Skeleton } from '@/components/ui/skeleton';
import { Suspense } from 'react';
import { Search } from '@/components/search';
export const metadata = { title: '搜索知识库' };
export default function SearchPage() {
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <h1>搜索知识库</h1>
          <p>检索当前版本，直接进入命中的正文位置。</p>
        </div>
      </div>
      <Suspense fallback={<Skeleton className="h-48 w-full" />}>
        <Search />
      </Suspense>
    </div>
  );
}
