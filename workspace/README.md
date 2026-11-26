# workspace

This is a Next.js application generated with
[Create Fumadocs](https://github.com/fuma-nama/fumadocs).

From the repository root, run `docker compose up -d --build --wait workspace`
to serve the docs at http://localhost:1130/docs.

Run development server:

```bash
npm run dev
# or
pnpm dev
# or
yarn dev
```

Open http://localhost:3000 with your browser to see the result.

## Explore

In the project, you can see:

- `lib/source.ts`: Code for content source adapter, [`loader()`](https://fumadocs.dev/docs/headless/source-api) provides the interface to access your content.
- `lib/layout.shared.tsx`: Shared options for layouts, optional but preferred to keep.

| Route                     | Description                                            |
| ------------------------- | ------------------------------------------------------ |
| `app/(home)`              | The route group for your landing page and other pages. |
| `app/docs`                | The documentation layout and pages.                    |
| `app/api/search/route.ts` | The Route Handler for search.                          |

### Fumadocs MDX

Collections are defined with the [Macro API](https://fumadocs.dev/docs/mdx/macro) in `lib/source.ts`.

The project overview and engineering rules are maintained in [ARCHITECTURE.md](ARCHITECTURE.md) and [PROJECT.md](PROJECT.md). Fumadocs publishes them at `/docs/architecture` and `/docs/project`. Topic documents keep their existing `/docs/<category>/<slug>` paths. The collection reads the two top-level files and `content/`, and GitHub links point to the source files under `workspace/`.

Standalone architecture attachments and their editable Archify sources are served from `public/design/`.

Read the [Introduction](https://fumadocs.dev/docs/mdx) for further details.

## Learn More

To learn more about Next.js and Fumadocs, take a look at the following
resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js
  features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.
- [Fumadocs](https://fumadocs.dev) - learn about Fumadocs
