import { describe, it, expect } from "vitest";
import {
  accessSync,
  constants,
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

/** Expose only the script's shell utilities, never an inherited ao launcher. */
function buildIsolatedPath(binDir: string): string {
  for (const name of [
    "bash",
    "cat",
    "cut",
    "dirname",
    "echo", // GNU xargs resolves its default command through PATH.
    "find",
    "grep",
    "head",
    "mkdir",
    "tr",
    "wc",
    "xargs",
  ]) {
    const source = ["/usr/bin", "/bin"]
      .map((dir) => join(dir, name))
      .find((path) => {
        try {
          accessSync(path, constants.X_OK);
          return true;
        } catch {
          return false;
        }
      });
    if (!source) throw new Error(`Required doctor fixture utility is unavailable: ${name}`);
    symlinkSync(source, join(binDir, name));
  }
  return binDir;
}

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../../..");
const scriptPath = join(repoRoot, "scripts", "ao-doctor.sh");

function writeExecutable(path: string, content: string): void {
  writeFileSync(path, content);
  chmodSync(path, 0o755);
}

function createFakeBinary(binDir: string, name: string, body: string): void {
  writeExecutable(join(binDir, name), `#!/bin/bash\nset -e\n${body}\n`);
}

function createHealthyRepo(tempRoot: string): string {
  const fakeRepo = join(tempRoot, "repo");
  mkdirSync(join(fakeRepo, "node_modules"), { recursive: true });
  mkdirSync(join(fakeRepo, "packages", "core", "dist"), { recursive: true });
  mkdirSync(join(fakeRepo, "packages", "cli", "dist"), { recursive: true });
  mkdirSync(join(fakeRepo, "packages", "agent-orchestrator", "bin"), { recursive: true });
  mkdirSync(join(fakeRepo, "packages", "web"), { recursive: true });
  writeFileSync(join(fakeRepo, "packages", "core", "dist", "index.js"), "export {};\n");
  writeFileSync(join(fakeRepo, "packages", "cli", "dist", "index.js"), "export {};\n");
  writeFileSync(
    join(fakeRepo, "packages", "agent-orchestrator", "bin", "ao.js"),
    '#!/usr/bin/env node\nconsole.log("0.1.0");\n',
  );
  chmodSync(join(fakeRepo, "packages", "agent-orchestrator", "bin", "ao.js"), 0o755);
  return fakeRepo;
}

function createHealthyPath(binDir: string): void {
  createFakeBinary(
    binDir,
    "node",
    'if [ "$1" = "--version" ]; then\n  printf "v20.11.1\\n"\n  exit 0\nfi\nexit 0',
  );
  createFakeBinary(
    binDir,
    "git",
    'if [ "$1" = "--version" ]; then\n  printf "git version 2.43.0\\n"\n  exit 0\nfi\nexit 0',
  );
  createFakeBinary(
    binDir,
    "pnpm",
    'if [ "$1" = "--version" ]; then\n  printf "9.15.4\\n"\n  exit 0\nfi\nexit 0',
  );
  createFakeBinary(
    binDir,
    "npm",
    'if [ "$1" = "bin" ]; then\n  printf "/tmp/npm-bin\\n"\n  exit 0\nfi\nexit 0',
  );
  createFakeBinary(
    binDir,
    "tmux",
    'if [ "$1" = "-V" ]; then\n  printf "tmux 3.4\\n"\n  exit 0\nfi\nif [ "$1" = "list-sessions" ]; then\n  exit 1\nfi\nexit 0',
  );
  createFakeBinary(
    binDir,
    "gh",
    'if [ "$1" = "--version" ]; then\n  printf "gh version 2.50.0\\n"\n  exit 0\nfi\nif [ "$1" = "auth" ] && [ "$2" = "status" ]; then\n  exit 0\nfi\nexit 0',
  );
  createFakeBinary(binDir, "ao", 'printf "/fake/ao\\n" >/dev/null\nexit 0');
}

describe("scripts/ao-doctor.sh", () => {
  it("reports a healthy install as PASS", () => {
    const tempRoot = mkdtempSync(join(tmpdir(), "ao-doctor-script-"));
    const fakeRepo = createHealthyRepo(tempRoot);
    const binDir = join(tempRoot, "bin");
    mkdirSync(binDir, { recursive: true });
    createHealthyPath(binDir);

    const configPath = join(tempRoot, "agent-orchestrator.yaml");
    const dataDir = join(tempRoot, "data");
    const worktreeDir = join(tempRoot, "worktrees");
    mkdirSync(dataDir, { recursive: true });
    mkdirSync(worktreeDir, { recursive: true });
    writeFileSync(
      configPath,
      [`dataDir: ${dataDir}`, `worktreeDir: ${worktreeDir}`, "projects: {}"].join("\n"),
    );

    const result = spawnSync("bash", [scriptPath], {
      env: {
        ...process.env,
        PATH: buildIsolatedPath(binDir),
        AO_REPO_ROOT: fakeRepo,
        AO_CONFIG_PATH: configPath,
      },
      encoding: "utf8",
      timeout: 20_000,
    });

    rmSync(tempRoot, { recursive: true, force: true });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("PASS");
    expect(result.stdout).toContain("Environment looks healthy");
  });

  it("applies safe fixes for missing launcher, missing dirs, and stale temp files", () => {
    const tempRoot = mkdtempSync(join(tmpdir(), "ao-doctor-fix-"));
    const fakeRepo = createHealthyRepo(tempRoot);
    const binDir = join(tempRoot, "bin");
    mkdirSync(binDir, { recursive: true });
    createHealthyPath(binDir);
    rmSync(join(binDir, "ao"), { force: true });

    const npmLog = join(tempRoot, "npm.log");
    createFakeBinary(
      binDir,
      "npm",
      `printf '%s\\n' "$*" >> ${JSON.stringify(npmLog)}\nif [ "$1" = "bin" ]; then\n  printf "/tmp/npm-bin\\n"\nfi\nexit 0`,
    );

    const configPath = join(tempRoot, "agent-orchestrator.yaml");
    const dataDir = join(tempRoot, "data");
    const worktreeDir = join(tempRoot, "worktrees");
    const commentedDataDir = `${dataDir} # session metadata`;
    const commentedWorktreeDir = `${worktreeDir} # ephemeral worktrees`;
    writeFileSync(
      configPath,
      [`dataDir: ${commentedDataDir}`, `worktreeDir: ${commentedWorktreeDir}`, "projects: {}"].join(
        "\n",
      ),
    );

    const tmpRoot = join(tempRoot, "tmp-root");
    mkdirSync(tmpRoot, { recursive: true });
    const staleFile = join(tmpRoot, "ao-stale.tmp");
    writeFileSync(staleFile, "stale\n");
    const oldTimestamp = new Date(Date.now() - 2 * 60 * 60 * 1000);
    utimesSync(staleFile, oldTimestamp, oldTimestamp);

    const result = spawnSync("bash", [scriptPath, "--fix"], {
      env: {
        ...process.env,
        PATH: buildIsolatedPath(binDir),
        AO_REPO_ROOT: fakeRepo,
        AO_CONFIG_PATH: configPath,
        AO_DOCTOR_TMP_ROOT: tmpRoot,
      },
      encoding: "utf8",
      timeout: 20_000,
    });

    const npmCommands = readFileSync(npmLog, "utf8");
    const staleStillExists = existsSync(staleFile);
    const dataDirExists = existsSync(dataDir);
    const worktreeDirExists = existsSync(worktreeDir);
    const commentedDataDirExists = existsSync(commentedDataDir);
    const commentedWorktreeDirExists = existsSync(commentedWorktreeDir);
    rmSync(tempRoot, { recursive: true, force: true });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("FIXED");
    expect(npmCommands).toContain("link");
    expect(result.stdout).toContain("launcher");
    expect(result.stdout).toContain("stale temp files");
    expect(staleStillExists).toBe(false);
    expect(dataDirExists).toBe(true);
    expect(worktreeDirExists).toBe(true);
    expect(commentedDataDirExists).toBe(false);
    expect(commentedWorktreeDirExists).toBe(false);
  });
});
