import { defineConfig } from "vitest/config";

// The tests drive a real Chrome, so they are slower than ordinary unit tests and
// they must not run at the same time as one another: two Chrome windows fighting
// over one display make the results depend on timing. One file at a time it is.
export default defineConfig({
  test: {
    include: ["test/**/*.test.ts"],
    testTimeout: 60_000,
    hookTimeout: 60_000,
    fileParallelism: false,
    coverage: {
      provider: "v8",
      include: ["src/**/*.ts"],
      reporter: ["text", "json-summary"],
      thresholds: {
        lines: 70,
        statements: 70,
        functions: 70,
        branches: 70,
      },
    },
  },
});
