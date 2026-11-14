'use client';

import Link from 'next/link';
import { fetchClient } from 'fumadocs-core/search/client/fetch';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { RefreshCw, Search as SearchIcon } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group';
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from '@/components/ui/item';
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select';
import { Skeleton } from '@/components/ui/skeleton';

interface Result {
  readonly url: string;
  readonly meta: { readonly title?: string };
  readonly excerpt: string;
  readonly plain?: boolean;
}
interface Pagefind {
  search(
    query: string,
    options: { filters?: Record<string, string> },
  ): Promise<{ results: Array<{ data(): Promise<Result> }> }>;
}
interface SearchResponse {
  readonly key: string;
  readonly state: 'ready' | 'error';
  readonly results: Result[];
}

/** The same-origin Fumadocs endpoint keeps private engine credentials on the server. */
async function searchOnline(
  query: string,
  category: string,
): Promise<Result[]> {
  const found = await fetchClient({
    api: `${process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? ''}/api/search/`,
    tag: category || undefined,
    cache: new Map(),
  }).search(query);
  const groups = new Map<string, Result>();
  for (const item of found) {
    const page = item.url.split('#')[0];
    if (item.type === 'page') {
      groups.set(page, {
        url: page,
        meta: { title: item.content },
        excerpt: '',
        plain: true,
      });
    } else {
      const group = groups.get(page);
      if (group && !group.excerpt) {
        groups.set(page, { ...group, url: item.url, excerpt: item.content });
      }
    }
  }
  return [...groups.values()];
}

/** Searches the frozen version; URL state survives refresh and browser history. */
export function Search() {
  const params = useSearchParams();
  const query = params.get('q') ?? '';
  const category = params.get('category') ?? '';
  const [retry, setRetry] = useState(0);
  const [response, setResponse] = useState<SearchResponse | null>(null);
  const requestKey = JSON.stringify([query, category, retry]);
  function updateParameter(name: string, value: string, push = false) {
    const url = new URL(location.href);
    if (value) {
      url.searchParams.set(name, value);
    } else {
      url.searchParams.delete(name);
    }
    if (push) {
      history.pushState(null, '', url);
    } else {
      history.replaceState(null, '', url);
    }
  }
  useEffect(() => {
    if (!query.trim()) {
      return;
    }
    let canceled = false;
    const timer = setTimeout(async () => {
      try {
        let results: Result[];
        if (process.env.NEXT_PUBLIC_WORKSPACE_MODE === 'online') {
          results = await searchOnline(query, category);
        } else {
          const url = `${process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? ''}/pagefind/pagefind.js`;
          const pagefind: Pagefind = await import(
            /* webpackIgnore: true */ url
          );
          const found = await pagefind.search(query, {
            filters: category ? { category } : undefined,
          });
          results = await Promise.all(
            found.results.slice(0, 30).map((result) => result.data()),
          );
        }
        if (!canceled) {
          setResponse({ key: requestKey, state: 'ready', results });
        }
      } catch {
        if (!canceled) {
          setResponse({ key: requestKey, state: 'error', results: [] });
        }
      }
    }, 200);
    return () => {
      canceled = true;
      clearTimeout(timer);
    };
  }, [query, category, retry, requestKey]);
  const state = !query.trim()
    ? 'idle'
    : response?.key === requestKey
      ? response.state
      : 'loading';
  const results = response?.key === requestKey ? response.results : [];
  return (
    <div className="flex flex-col gap-6">
      <FieldGroup className="grid items-end gap-4 sm:grid-cols-[minmax(0,1fr)_12rem]">
        <Field>
          <FieldLabel htmlFor="knowledge-search">全文搜索</FieldLabel>
          <InputGroup>
            <InputGroupAddon>
              <SearchIcon aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              id="knowledge-search"
              name="knowledge-search"
              type="search"
              placeholder="中文关键词、FR 或 SRC 编号…"
              value={query}
              onChange={(event) => updateParameter('q', event.target.value)}
              autoComplete="off"
            />
          </InputGroup>
        </Field>
        <Field>
          <FieldLabel htmlFor="search-category">文档分类</FieldLabel>
          <NativeSelect
            id="search-category"
            value={category}
            onChange={(event) =>
              updateParameter('category', event.target.value, true)
            }
            className="w-full"
          >
            <NativeSelectOption value="">全部分类</NativeSelectOption>
            {[
              '原始需求',
              '设计方案',
              '产品需求',
              '执行计划',
              '调研资料',
              '文档模板',
              'Skill',
              '工作流',
            ].map((name) => (
              <NativeSelectOption key={name}>{name}</NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
      </FieldGroup>
      <p
        aria-live="polite"
        role="status"
        className="text-sm text-muted-foreground"
      >
        {state === 'ready'
          ? `显示 ${results.length} 条结果`
          : state === 'loading'
            ? '正在搜索当前知识版本…'
            : ''}
      </p>
      {state === 'loading' && (
        <div aria-hidden="true" className="flex flex-col gap-4">
          {[0, 1, 2].map((key) => (
            <Skeleton key={key} className="h-24 w-full" />
          ))}
        </div>
      )}
      {state === 'idle' && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>从一个关键词开始</EmptyTitle>
            <EmptyDescription>支持中文、文档标题和业务编号。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {state === 'error' && (
        <Alert>
          <AlertTitle>搜索暂时不可用</AlertTitle>
          <AlertDescription>
            请稍后重试，文档仍可从目录阅读。
            <Button
              variant="outline"
              size="sm"
              onClick={() => setRetry((value) => value + 1)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              重试
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {state === 'ready' && !results.length && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>没有找到相关文档</EmptyTitle>
            <EmptyDescription>
              试试更短的关键词，或选择全部分类。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {state === 'ready' && (
        <ItemGroup>
          {results.map((result) => (
            <div key={result.url} role="listitem">
              <Item className="search-result" asChild variant="outline">
                <Link href={result.url}>
                  <ItemContent className="min-w-0">
                    <ItemTitle>{result.meta.title ?? result.url}</ItemTitle>
                    <ItemDescription>
                      {result.plain ? (
                        result.excerpt
                      ) : (
                        <span
                          dangerouslySetInnerHTML={{ __html: result.excerpt }}
                        />
                      )}
                    </ItemDescription>
                  </ItemContent>
                </Link>
              </Item>
            </div>
          ))}
        </ItemGroup>
      )}
    </div>
  );
}
