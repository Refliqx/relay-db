# RelayDB marketing site

Standalone Vite + Svelte 5 landing page for RelayDB. It shares
`../design/tokens.css` with the desktop app, so colour, type and radius stay in
sync, but it is built and deployed independently of the Tauri bundle.

## Develop

```bash
bun install
bun run dev
```

## Build

```bash
bun run build     # writes dist/
bun run preview   # serve the production build locally
```

## Deploy

Any static host works. On Vercel, set the project root to `website/`, keep the
Vite preset, and use `dist` as the output directory.

The site links to GitHub for releases and documentation. If the repository
moves, update `src/lib/links.ts`.
