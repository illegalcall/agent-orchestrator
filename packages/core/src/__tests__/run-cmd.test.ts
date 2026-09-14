import { describe, expect, it } from "vitest";
import { runCmd, tryRunCmd } from "../utils/run-cmd.js";

describe("shared subprocess execution", () => {
  it("returns separate output streams and preserves leading whitespace", async () => {
    const output = await runCmd(process.execPath, [
      "-e",
      "process.stdout.write('  output \\n'); process.stderr.write('error\\n')",
    ]);
    expect(output).toEqual({ stdout: "  output", stderr: "error" });
  });
  it("passes argv literally without invoking a shell", async () => {
    const literal = "$(echo unwanted); spaced argument";
    expect(
      (await runCmd(process.execPath, ["-e", "process.stdout.write(process.argv[1])", literal]))
        .stdout,
    ).toBe(literal);
  });
  it("merges the requested environment and working directory", async () => {
    const { stdout } = await runCmd(
      process.execPath,
      [
        "-e",
        "console.log(JSON.stringify([process.cwd(),process.env.RUN_CMD_TEST,Boolean(process.env.PATH)]))",
      ],
      { cwd: process.cwd(), env: { RUN_CMD_TEST: "sentinel" } },
    );
    expect(JSON.parse(stdout)).toEqual([process.cwd(), "sentinel", true]);
  });
  it("honors an explicit timeout and turns failures into null only in tryRunCmd", async () => {
    const args = ["-e", "setTimeout(() => {}, 10000)"];
    await expect(runCmd(process.execPath, args, { timeout: 30 })).rejects.toMatchObject({
      killed: true,
    });
    await expect(tryRunCmd(process.execPath, args, { timeout: 30 })).resolves.toBeNull();
    await expect(runCmd(process.execPath, ["-e", "process.exit(4)"])).rejects.toMatchObject({
      code: 4,
    });
  });
  it("accepts zero as an explicitly disabled timeout", async () => {
    await expect(
      runCmd(process.execPath, ["-e", "setTimeout(() => console.log('done'), 40)"], { timeout: 0 }),
    ).resolves.toEqual({ stdout: "done", stderr: "" });
  });
});
