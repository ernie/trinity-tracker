import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";
import eslintPluginPrettierRecommended from "eslint-plugin-prettier/recommended";
import { defineConfig, globalIgnores } from "eslint/config";

export default defineConfig([
  // public/ holds vendored engine artifacts (minified trinity.js etc.) —
  // keep them out of both lint and Prettier's reach.
  globalIgnores(["dist", "public"]),
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
  // Runs Prettier as an ESLint rule, so formatting drift fails the lint
  // gate (which `bun run build` already enforces). Must come last:
  // eslint-config-prettier (bundled in this preset) turns off rules that
  // would conflict with Prettier.
  eslintPluginPrettierRecommended,
]);
