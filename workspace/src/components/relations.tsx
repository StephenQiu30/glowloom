'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { ArrowDown } from 'lucide-react';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select';
import { DocumentList } from '@/components/document-list';

interface KnowledgeNode {
  readonly id: string;
  readonly title: string;
  readonly category: string;
  readonly sourceIds: string[];
  readonly references: string[];
}
/** Distinguishes the declared source chain from ordinary cross references. */
export function Relations({ nodes }: { readonly nodes: KnowledgeNode[] }) {
  const params = useSearchParams();
  const selected =
    params.get('document') ??
    nodes.find((document) => document.id === 'plan-38')?.id ??
    nodes[0]?.id ??
    '';
  const document = nodes.find((node) => node.id === selected);
  const chain: KnowledgeNode[] = [];
  const seen = new Set<string>();
  let current = document;
  while (current && !seen.has(current.id)) {
    seen.add(current.id);
    chain.unshift(current);
    const sourceId = current.sourceIds[0];
    current = nodes.find((node) => node.id === sourceId);
  }
  const incoming = nodes.filter((node) => node.sourceIds.includes(selected));
  const references = nodes.filter(
    (node) =>
      document?.references.includes(node.id) &&
      !document.sourceIds.includes(node.id),
  );
  return (
    <div className="flex flex-col gap-8">
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="relation-document">选择文档</FieldLabel>
          <NativeSelect
            id="relation-document"
            value={selected}
            onChange={(event) => {
              const url = new URL(location.href);
              url.searchParams.set('document', event.target.value);
              history.pushState(null, '', url);
            }}
            className="w-full"
          >
            <NativeSelectOption value="" disabled>
              请选择文档
            </NativeSelectOption>
            {nodes.map((node) => (
              <NativeSelectOption value={node.id} key={node.id}>
                {node.title}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
      </FieldGroup>
      <section
        aria-label="唯一来源链"
        className="mx-auto flex w-full max-w-2xl flex-col items-center gap-3"
      >
        {chain.map((node, index) => (
          <div
            key={node.id}
            className="flex w-full flex-col items-center gap-3"
          >
            {index > 0 && (
              <ArrowDown
                aria-hidden="true"
                className="size-4 text-muted-foreground"
              />
            )}
            <Card className="w-full">
              <CardHeader>
                <CardDescription>{node.category}</CardDescription>
                <CardTitle>
                  <Link className="hover:underline" href={`/read/${node.id}/`}>
                    {node.title}
                  </Link>
                </CardTitle>
              </CardHeader>
            </Card>
          </div>
        ))}
      </section>
      {[
        { title: '由此文档承接', documents: incoming },
        { title: '其他引用', documents: references },
      ].map((group) => (
        <Card key={group.title}>
          <CardHeader>
            <CardTitle>{group.title}</CardTitle>
          </CardHeader>
          <CardContent>
            {group.documents.length ? (
              <DocumentList documents={group.documents} />
            ) : (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>暂无{group.title}</EmptyTitle>
                </EmptyHeader>
              </Empty>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
