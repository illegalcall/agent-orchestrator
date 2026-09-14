import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@composio/ao-core", () => ({ runCmd: vi.fn(), tryRunCmd: vi.fn() }));
import { runCmd, tryRunCmd } from "@composio/ao-core";
import { exec, execSilent } from "../src/lib/shell.js";

beforeEach(() => vi.resetAllMocks());

describe("CLI subprocess policy", () => {
  it("keeps network commands unbounded while forwarding cwd and environment", async () => {
    vi.mocked(runCmd).mockResolvedValue({ stdout: "cloned", stderr: "" });
    await expect(
      exec("git", ["clone", "example"], { cwd: "/workspace", env: { TEST: "1" } }),
    ).resolves.toEqual({ stdout: "cloned", stderr: "" });
    expect(runCmd).toHaveBeenCalledWith("git", ["clone", "example"], {
      cwd: "/workspace",
      env: { TEST: "1" },
      timeout: 0,
    });
  });
  it("allows a caller to select a bounded timeout", async () => {
    await exec("git", ["status"], { timeout: 2500 });
    expect(runCmd).toHaveBeenCalledWith("git", ["status"], { timeout: 2500 });
  });
  it("preserves the unbounded silent-command policy and null failure result", async () => {
    vi.mocked(tryRunCmd).mockResolvedValue(null);
    await expect(execSilent("gh", ["repo", "view"])).resolves.toBeNull();
    expect(tryRunCmd).toHaveBeenCalledWith("gh", ["repo", "view"], { timeout: 0 });
  });
});
