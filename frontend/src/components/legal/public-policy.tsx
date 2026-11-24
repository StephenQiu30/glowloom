import Link from "next/link";
import { PageHeader } from "@/components/layout/page-header";
import { ArrowLeft, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type LegalDocument = {
  title: string;
  sha256: string;
  text: string;
};

// The versioned policy package contains paragraphs, headings, tables and links.
function inline(text: string) {
  return text.split(/(`[^`]+`|\[[^\]]+\]\([^)]+\))/u).map((part, i) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return <code key={i}>{part.slice(1, -1)}</code>;
    }
    const link = part.match(/^\[([^\]]+)\]\(([^)]+)\)$/u);
    if (!link) return part;
    const [, title, target] = link;
    if (target === "privacy.md" || target === "terms.md") {
      return (
        <Link key={i} href={target === "privacy.md" ? "/privacy" : "/terms"}>
          {title}
        </Link>
      );
    }
    if (target.startsWith("mailto:")) {
      return (
        <a key={i} href={target}>
          {title}
        </a>
      );
    }
    return part;
  });
}

function cells(row: string) {
  return row
    .trim()
    .slice(1, -1)
    .split("|")
    .map((cell) => cell.trim());
}

function PolicyBody({ text }: { text: string }) {
  return text
    .trim()
    .split(/\n\n+/u)
    .map((block) => {
      if (block.startsWith("# ")) return null;
      if (block.startsWith("## ")) return <h2 key={block}>{block.slice(3)}</h2>;
      if (!block.startsWith("|")) return <p key={block}>{inline(block)}</p>;

      const [header, , ...rows] = block.split("\n");
      return (
        <Table className="policy-table" key={header}>
          <TableHeader>
            <TableRow>
              {cells(header).map((value) => (
                <TableHead key={value} scope="col">
                  {inline(value)}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row}>
                {cells(row).map((value, column) => (
                  <TableCell key={column}>{inline(value)}</TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      );
    });
}

export function PublicPolicy({
  document,
  version,
  filename,
}: {
  document: LegalDocument;
  version: string;
  filename: "terms.md" | "privacy.md";
}) {
  return (
    <main className="public-policy">
      <div className="workspace-section-heading">
        <Button variant="ghost" asChild>
          <Link href="/">
            <ArrowLeft aria-hidden="true" data-icon="inline-start" />
            返回织光
          </Link>
        </Button>
        <Button variant="outline" asChild>
          <a href={`/legal/${version}/${filename}`} download>
            <Download aria-hidden="true" data-icon="inline-start" />
            保存本文
          </a>
        </Button>
      </div>
      <PageHeader
        title={document.title}
        description={`协议包 ${version} · 本地审阅版`}
      />
      <Separator />
      <article className="policy-body">
        <PolicyBody text={document.text} />
      </article>
      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="ghost">查看文件校验值</Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <p className="px-2 text-sm break-all text-muted-foreground">
            SHA-256 {document.sha256}
          </p>
        </CollapsibleContent>
      </Collapsible>
    </main>
  );
}
