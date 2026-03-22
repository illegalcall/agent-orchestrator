/**
 * Shared command execution utility.
 *
 * Wraps Node.js execFile with consistent timeout, buffering, and error
 * handling. Prefer this over raw child_process calls so that every
 * `execFile` invocation in the codebase shares the same defaults.
 */

import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

/** Default timeout for external commands (30 seconds). */
const DEFAULT_TIMEOUT_MS = 30_000;
/** Default max output buffer size (10 MB). */
const DEFAULT_MAX_BUFFER = 10 * 1024 * 1024;

export interface RunCmdOptions {
  /** Working directory for the subprocess. */
  cwd?: string;
  /** Extra environment variables merged on top of process.env. */
  env?: Record<string, string>;
  /**
   * Execution timeout in milliseconds.
   * Defaults to 30 000 ms (30 s).
   */
  timeout?: number;
  /**
   * Maximum stdout/stderr buffer size in bytes.
   * Defaults to 10 MB.
   */
  maxBuffer?: number;
}

export interface RunCmdResult {
  stdout: string;
  stderr: string;
}

/**
 * Run an external command via execFile (no shell injection risk).
 *
 * Returns stdout and stderr trimmed of trailing whitespace.
 * Throws on non-zero exit code.
 */
export async function runCmd(
  cmd: string,
  args: string[],
  options?: RunCmdOptions,
): Promise<RunCmdResult> {
  const { stdout, stderr } = await execFileAsync(cmd, args, {
    cwd: options?.cwd,
    env: options?.env ? { ...process.env, ...options.env } : undefined,
    timeout: options?.timeout ?? DEFAULT_TIMEOUT_MS,
    maxBuffer: options?.maxBuffer ?? DEFAULT_MAX_BUFFER,
  });
  return { stdout: stdout.trimEnd(), stderr: stderr.trimEnd() };
}

/**
 * Run a command, returning stdout or `null` on any error (non-zero
 * exit, timeout, ENOENT, etc.).
 */
export async function tryRunCmd(
  cmd: string,
  args: string[],
  options?: RunCmdOptions,
): Promise<string | null> {
  try {
    const { stdout } = await runCmd(cmd, args, options);
    return stdout;
  } catch {
    return null;
  }
}
