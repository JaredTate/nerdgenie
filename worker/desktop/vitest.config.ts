import { defineConfig } from "vitest/config"

// The coverage floor is seventy percent, which is what docs/WORK_PLAN.md Part 1
// asks of the two TypeScript workers under "Definition of done".
export default defineConfig({
  test: {
    include: ["test/**/*.test.ts"],
    testTimeout: 60_000,
    hookTimeout: 60_000,
    // The fixture-window tests drive one real window at a time, so nothing here
    // may run beside anything else.
    fileParallelism: false,
    coverage: {
      provider: "v8",
      include: ["src/**/*.ts"],
      // The process wiring is proved by starting the worker for real in the
      // fixture tests and by the Go side's integration test, not by counting
      // lines, so it is measured but not part of the floor.
      exclude: ["src/main.ts"],
      reporter: ["text", "json-summary"],
      thresholds: {
        lines: 70,
        statements: 70,
        functions: 70,
        branches: 70,
      },
    },
  },
})
