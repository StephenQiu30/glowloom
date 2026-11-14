'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

/** Protects local edits across document navigation and browser unload. */
export function DraftGuard({ dirty }: { readonly dirty: boolean }) {
  const router = useRouter();
  const [target, setTarget] = useState('');
  const leaving = useRef(false);
  useEffect(() => {
    if (!dirty) {
      return;
    }
    function beforeUnload(event: BeforeUnloadEvent) {
      if (!leaving.current) {
        event.preventDefault();
        event.returnValue = '';
      }
    }
    function onClick(event: MouseEvent) {
      if (
        leaving.current ||
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey ||
        !(event.target instanceof Element)
      ) {
        return;
      }
      const link = event.target.closest<HTMLAnchorElement>('a[href]');
      if (!link || link.target === '_blank' || link.hasAttribute('download')) {
        return;
      }
      const url = new URL(link.href);
      if (
        url.origin !== location.origin ||
        (url.pathname === location.pathname && url.search === location.search)
      ) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      setTarget(url.pathname + url.search + url.hash);
    }
    function onCommand(event: Event) {
      if (!(event instanceof CustomEvent)) {
        return;
      }
      const detail: unknown = event.detail;
      if (
        detail &&
        typeof detail === 'object' &&
        'href' in detail &&
        typeof detail.href === 'string'
      ) {
        event.preventDefault();
        setTarget(detail.href);
      }
    }
    document.addEventListener('workspace:navigate', onCommand);
    window.addEventListener('beforeunload', beforeUnload);
    document.addEventListener('click', onClick, true);
    return () => {
      document.removeEventListener('workspace:navigate', onCommand);
      window.removeEventListener('beforeunload', beforeUnload);
      document.removeEventListener('click', onClick, true);
    };
  }, [dirty]);
  return (
    <AlertDialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) {
          setTarget('');
        }
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>离开未保存的文档？</AlertDialogTitle>
          <AlertDialogDescription>
            当前编辑尚未保存为提案。留在本页可继续编辑或保存。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>继续编辑</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              leaving.current = true;
              router.push(target);
            }}
          >
            放弃修改并离开
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
