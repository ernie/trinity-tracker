import { describe, expect, test, beforeEach } from "bun:test";
import {
  loadPlatform,
  savePlatform,
  PLATFORM_STORAGE_KEY,
} from "./platformStorage";

// Bun's test runtime doesn't include localStorage out of the box —
// a minimal in-memory shim is enough for these unit tests.
const store: Record<string, string> = {};
(globalThis as { localStorage?: Storage }).localStorage = {
  getItem: (k: string) => (k in store ? store[k] : null),
  setItem: (k: string, v: string) => {
    store[k] = v;
  },
  removeItem: (k: string) => {
    delete store[k];
  },
  clear: () => {
    for (const k in store) delete store[k];
  },
  key: () => null,
  length: 0,
} as Storage;

describe("platformStorage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test("loadPlatform returns null when nothing stored", () => {
    expect(loadPlatform()).toBe(null);
  });

  test("loadPlatform returns null when stored value is invalid", () => {
    localStorage.setItem(PLATFORM_STORAGE_KEY, "mars-rover");
    expect(loadPlatform()).toBe(null);
  });

  test("loadPlatform returns each valid platform", () => {
    localStorage.setItem(PLATFORM_STORAGE_KEY, "flatscreen");
    expect(loadPlatform()).toBe("flatscreen");
    localStorage.setItem(PLATFORM_STORAGE_KEY, "pcvr");
    expect(loadPlatform()).toBe("pcvr");
    localStorage.setItem(PLATFORM_STORAGE_KEY, "quest");
    expect(loadPlatform()).toBe("quest");
  });

  test("savePlatform writes the value and survives a load", () => {
    savePlatform("quest");
    expect(loadPlatform()).toBe("quest");
  });

  test("savePlatform overwrites an earlier value", () => {
    savePlatform("flatscreen");
    savePlatform("pcvr");
    expect(loadPlatform()).toBe("pcvr");
  });
});
