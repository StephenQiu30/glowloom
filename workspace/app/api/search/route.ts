import { numberedDocumentTitle, searchableSource } from '@/lib/source';
import { createFromSource } from 'fumadocs-core/search/server';

export const { GET } = createFromSource(searchableSource, {
  buildIndex(page) {
    const { title, description, structuredData } = page.data;
    const firstHeading = structuredData.headings[0];
    const sourceHeading =
      numberedDocumentTitle(page.path) &&
      firstHeading?.content === page.data.sourceHeading
        ? firstHeading
        : undefined;

    return {
      id: page.url,
      url: page.url,
      title,
      description,
      structuredData: sourceHeading
        ? {
            headings: structuredData.headings.slice(1),
            contents: structuredData.contents.map((entry) =>
              entry.heading === sourceHeading.id
                ? { ...entry, heading: undefined }
                : entry,
            ),
          }
        : structuredData,
    };
  },
});
