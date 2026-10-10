import { defineConfig, configDefaults } from "vitest/config";
import { fileURLToPath } from "node:url";

// Match Next.js/tsconfig '@/*' imports when testing App Router handlers.
export default defineConfig({
  test: { exclude: [...configDefaults.exclude, "e2e/**"] },
  resolve: {
    alias: {
      "@": fileURLToPath(new URL(".", import.meta.url)),
    },
  },
});
