import { expect, test } from "bun:test";
import { assistMessage } from "./assistMessage";

test("phrases each mod assist token for its mode", () => {
  expect(assistMessage("frag")).toBe("assisted a capture!");
  expect(assistMessage("return")).toBe("assisted a capture!");
  expect(assistMessage("carry")).toBe("assisted a capture!");
  expect(assistMessage("skull")).toBe("assisted a skull delivery!");
  expect(assistMessage("obelisk")).toBe("helped destroy the obelisk!");
});

test("falls back generically for unknown or missing tokens", () => {
  expect(assistMessage(undefined)).toBe("earned an assist!");
  expect(assistMessage("future-token")).toBe("earned an assist!");
});
