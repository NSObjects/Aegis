import { defineConfig, configDefaults } from "vitest/config";
import { fileURLToPath } from "node:url";

// Match Next.js/tsconfig '@/*' imports when testing App Router handlers.
export default defineConfig({
  test: { exclude: [...configDefaults.exclude, "e2e/**", "e2e-local/**", "e2e-real-go/**"] },
  resolve: {
    alias: {
      "@": fileURLToPath(new URL(".", import.meta.url)),
    },
  },
});
