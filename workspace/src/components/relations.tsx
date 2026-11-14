"use client";
import { useState } from "react";
import Link from "next/link";
import { ArrowDown } from "lucide-react";
type Node = {
  id: string;
  title: string;
  category: string;
  sourceIds: string[];
  references: string[];
};
export function Relations({ nodes }: { nodes: Node[] }) {
  const [selected, setSelected] = useState(
    nodes.find((d) => d.id === "plan-38")?.id ?? nodes[0]?.id ?? "",
  );
  const doc = nodes.find((d) => d.id === selected);
  const chain: Node[] = [];
  const seen = new Set<string>();
  let current = doc;
  while (current && !seen.has(current.id)) {
    seen.add(current.id);
    chain.unshift(current);
    current = nodes.find((d) => d.id === current?.sourceIds[0]);
  }
  const incoming = nodes.filter((d) => d.sourceIds.includes(selected));
  return (
    <>
      <div className="search-filters">
        <label htmlFor="relation-document">选择文档</label>
        <select
          id="relation-document"
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
          style={{ maxWidth: "100%" }}
        >
          {nodes.map((d) => (
            <option value={d.id} key={d.id}>
              {d.title}
            </option>
          ))}
        </select>
      </div>
      <div className="relation-flow" aria-label="唯一来源链">
        {chain.flatMap((d, i) => [
          i > 0 ? <ArrowDown key={`arrow-${d.id}`} aria-hidden="true" /> : null,
          <Link key={d.id} className="relation-node" href={`/read/${d.id}/`}>
            <strong>{d.title}</strong>
            <span>{d.category}</span>
          </Link>,
        ])}
      </div>
      <h2 className="section-title">由此文档承接</h2>
      <div className="document-list">
        {incoming.length ? (
          incoming.map((d) => (
            <Link className="document-row" href={`/read/${d.id}/`} key={d.id}>
              <div>
                <h3>{d.title}</h3>
                <p>唯一来源关系</p>
              </div>
            </Link>
          ))
        ) : (
          <p className="subtle-note">没有声明以本文为唯一来源的文档。</p>
        )}
      </div>
      <h2 className="section-title" style={{ marginTop: 32 }}>
        其他引用
      </h2>
      <div className="document-list">
        {doc?.references
          .filter((id) => !doc.sourceIds.includes(id))
          .map((id) => nodes.find((d) => d.id === id))
          .filter(Boolean)
          .map((d) => (
            <Link key={d!.id} className="document-row" href={`/read/${d!.id}/`}>
              <div>
                <h3>{d!.title}</h3>
                <p>普通资料引用</p>
              </div>
            </Link>
          ))}
      </div>
    </>
  );
}
