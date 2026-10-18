import { describe, expect, test } from "bun:test";
import { formatExitReason, classifyScores } from "./format";

describe("formatExitReason", () => {
  test("maps the game's and collector's reasons to prose", () => {
    expect(formatExitReason("Fraglimit hit.")).toBe("Frag limit");
    expect(formatExitReason("Capturelimit hit.")).toBe("Capture limit");
    expect(formatExitReason("Timelimit hit.")).toBe("Time limit");
    expect(formatExitReason("Overtime timelimit hit.")).toBe(
      "Overtime time limit",
    );
    expect(formatExitReason("Server shutdown.")).toBe("Server shut down");
    expect(formatExitReason("shutdown")).toBe("Server shut down");
    expect(formatExitReason("crashed")).toBe("Server crashed");
    expect(formatExitReason("Rotation.")).toBe("Map rotation");
    expect(formatExitReason("Vote passed.")).toBe("Vote passed");
  });
  test("falls back to titlecased input for unknown reasons", () => {
    expect(formatExitReason("weird_thing")).toBe("Weird thing");
  });
  test("handles empty/undefined", () => {
    expect(formatExitReason("")).toBe("");
    expect(formatExitReason(undefined)).toBe("");
  });
});

describe("classifyScores", () => {
  test("detects no-contest when both scores are <= 0", () => {
    expect(classifyScores(0, 0)).toBe("no_contest");
    expect(classifyScores(-1, 0)).toBe("no_contest");
  });
  test("detects tie when scores are equal and positive", () => {
    expect(classifyScores(22, 22)).toBe("tie");
    expect(classifyScores(100, 100)).toBe("tie");
  });
  test("detects winner", () => {
    expect(classifyScores(25, 22)).toBe("left");
    expect(classifyScores(22, 25)).toBe("right");
  });
});
