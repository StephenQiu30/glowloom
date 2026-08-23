import '../tailwind.css';

if (typeof document !== 'undefined') {
  document.documentElement.lang = 'zh-CN';
  const viewport = document.querySelector<HTMLMetaElement>(
    'meta[name="viewport"]',
  );
  if (viewport) {
    viewport.content = 'width=device-width, initial-scale=1';
  }
}
