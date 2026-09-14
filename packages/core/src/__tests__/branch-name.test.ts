import { describe, expect, it } from "vitest";
import { getFallbackBranchName } from "../utils/branch-name.js";

describe("spawn branch fallback", () => {
  it.each([
    [undefined, "session/app-1"],
    ["INT-42", "feat/INT-42"],
    ["#42", "feat/42"],
    ["fix login bug", "feat/fix-login-bug"],
    ["two..dots", "feat/two-dots"],
    ["!!!", "feat/app-1"],
  ])("preserves branch naming for %s", (issue, expected) => {
    expect(getFallbackBranchName(issue, "app-1")).toBe(expected);
  });
});
