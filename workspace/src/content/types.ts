export interface Heading {
  id: string;
  text: string;
  depth: number;
}
export interface TextBlock {
  id: string;
  text: string;
  start: number;
  end: number;
  heading: string;
}
export interface Diagram {
  id: string;
  source: string;
}
export interface ParsedDocument {
  title: string;
  html: string;
  headings: Heading[];
  blocks: TextBlock[];
  links: string[];
  diagrams: Diagram[];
}
