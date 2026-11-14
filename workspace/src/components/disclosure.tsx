import type { ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';

/** Keeps long evidence and comparisons keyboard-accessible and initially compact. */
export function Disclosure({
  label,
  children,
}: {
  readonly label: ReactNode;
  readonly children: ReactNode;
}) {
  return (
    <Collapsible className="group/disclosure">
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="max-w-full">
          <span className="truncate">{label}</span>
          <ChevronDown
            data-icon="inline-end"
            aria-hidden="true"
            className="transition-transform group-data-[state=open]/disclosure:rotate-180"
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-3">{children}</CollapsibleContent>
    </Collapsible>
  );
}
