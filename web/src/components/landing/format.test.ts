import { describe, expect, test } from "bun:test";
import { plural } from "./format";

describe("plural", () => {
  test("returns singular when n=1", () => {
    expect(plural(1, "player", "players")).toBe("player");
  });
  test("returns plural when n=0", () => {
    expect(plural(0, "player", "players")).toBe("players");
  });
  test("returns plural when n=2", () => {
    expect(plural(2, "arena", "arenas")).toBe("arenas");
  });
});
