import { createGetUrl } from 'fumadocs-core/source';

export const appName = '织光 Workspace';
export const docsRoute = '/read';
export const docsImageRoute = '/og/read';

// fill this with your actual GitHub info, for example:
export const gitConfig = {
  user: 'StephenQiu30',
  repo: 'glowloom',
  branch: 'main',
};

const getImageUrl = createGetUrl(docsImageRoute);

export function getPageImageUrl(page: { slugs: string[]; locale?: string }) {
  const segments = [...page.slugs, 'image.png'];

  return { segments, url: getImageUrl(segments, page.locale) };
}
