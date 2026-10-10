import { defineConfig } from "vitest/config";
import { fileURLToPath } from "node:url";

// Match Next.js/tsconfig '@/*' imports when testing App Router handlers.
export default defineConfig({
  resolve: {
    alias: {
      "@": fileURLToPath(new URL(".", import.meta.url)),
    },
  },
});
