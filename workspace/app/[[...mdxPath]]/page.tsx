import { generateStaticParamsFor, importPage } from "nextra/pages";
import { useMDXComponents as getMDXComponents } from "../../mdx-components";

type PageProps = {
  params: Promise<{ mdxPath?: string[] }>;
};

export const generateStaticParams = generateStaticParamsFor("mdxPath");

export async function generateMetadata({ params }: PageProps) {
  const { mdxPath } = await params;
  const { metadata } = await importPage(mdxPath);
  return metadata;
}

const { wrapper: Wrapper } = getMDXComponents();

export default async function Page({ params }: PageProps) {
  const resolvedParams = await params;
  const {
    default: MDXContent,
    toc,
    metadata,
    sourceCode,
  } = await importPage(resolvedParams.mdxPath);

  return (
    <Wrapper toc={toc} metadata={metadata} sourceCode={sourceCode}>
      <MDXContent params={resolvedParams} />
    </Wrapper>
  );
}
