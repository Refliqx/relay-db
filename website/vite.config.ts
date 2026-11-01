import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  server: {
    // design/tokens.css lives one level above this project, so the dev server
    // has to be allowed to read the repository root.
    fs: {
      allow: [
        fileURLToPath(new URL(".", import.meta.url)),
        fileURLToPath(new URL("..", import.meta.url))
      ]
    }
  },
  build: { target: "es2022" }
});
