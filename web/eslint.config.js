import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";
import eslintPluginPrettierRecommended from "eslint-plugin-prettier/recommended";
import { defineConfig, globalIgnores } from "eslint/config";

export default defineConfig([
  // public/ holds vendored engine builds.
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
  // Prettier runs as a lint rule, so formatting drift fails the build.
  // Last, so its bundled eslint-config-prettier can turn off conflicting
  // rules.
  eslintPluginPrettierRecommended,
]);
