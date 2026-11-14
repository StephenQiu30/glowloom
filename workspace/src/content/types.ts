export type Heading = { id: string; text: string; depth: number };
export type TextBlock = {
  id: string;
  text: string;
  start: number;
  end: number;
  heading: string;
};
export type Diagram = { id: string; source: string };
export type ParsedDocument = {
  title: string;
  html: string;
  headings: Heading[];
  blocks: TextBlock[];
  links: string[];
  diagrams: Diagram[];
};
