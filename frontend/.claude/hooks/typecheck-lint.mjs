// PostToolUse hook: lint the edited file + type-check the project.
// Runs after Edit/Write/MultiEdit. Exit code 2 feeds problems back to Claude.
import { execSync } from "node:child_process";
import { existsSync } from "node:fs";

let raw = "";
process.stdin.setEncoding("utf8");
for await (const chunk of process.stdin) raw += chunk;

let data = {};
try {
  data = JSON.parse(raw || "{}");
} catch {
  process.exit(0);
}

const filePath = data?.tool_input?.file_path ?? "";
const normalized = filePath.replace(/\\/g, "/");

// Only care about TS/TSX source files.
if (!/\.(ts|tsx)$/.test(normalized) || !normalized.includes("/src/")) {
  process.exit(0);
}

const problems = [];

// Lint just the edited file (fast) — only if a flat ESLint config exists.
const hasEslintConfig = ["eslint.config.js", "eslint.config.mjs", "eslint.config.cjs"].some(
  (f) => existsSync(f)
);
if (hasEslintConfig) {
  try {
    execSync(`pnpm exec eslint "${filePath}" --max-warnings 0`, { stdio: "pipe" });
  } catch (e) {
    problems.push("ESLint:\n" + (e.stdout?.toString() || e.message));
  }
}

// Type-check the whole project (path aliases + project references).
try {
  execSync("pnpm run type-check", { stdio: "pipe" });
} catch (e) {
  problems.push("TypeScript:\n" + (e.stdout?.toString() || e.message));
}

if (problems.length) {
  console.error(problems.join("\n\n"));
  process.exit(2);
}
process.exit(0);
