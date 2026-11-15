# Fumadocs

This site is built with [Fumadocs](https://fumadocs.dev) on Next.js. The APIs and conventions of Fumadocs may differ from your training data, so check its docs for the task at hand:

- Writing pages: https://fumadocs.dev/docs/next/markdown.md
- Organizing pages and folders: https://fumadocs.dev/docs/next/page-conventions.md
- Customizing the UI: https://fumadocs.dev/docs/next/guides/customize-ui.md
- Changing the theme and colors: https://fumadocs.dev/docs/ui/theme.md
- Upgrading Fumadocs: https://fumadocs.dev/docs/next/upgrade.md
- Other tasks: https://fumadocs.dev/llms.txt lists every docs page.

## Project constraints

- Inherit the repository rules. Own TypeScript follows Google TS; use the official Fumadocs UI and shadcn theme rather than recreating document components.
- Keep the existing content files and stable document identities. Content is ordinary Markdown; preserve original bodies and use the official MDX collection and extension APIs.
- One anonymous knowledge entry on port 1130. No custom authentication, PG, workflow executor, host bridge, TTS or duplicated static/online applications.
- Use the official scaffold and search synchronization. Do not recreate the retired src, scripts, release-builder or compatibility-renderer architecture.
- Keep runtime credentials and Meilisearch data private and outside Git. Run lint, types, format, build and browser acceptance after changes.
