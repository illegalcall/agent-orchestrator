import { runCmd, tryRunCmd } from "@composio/ao-core";

export type { RunCmdResult as ExecResult } from "@composio/ao-core";

export async function exec(
  cmd: string,
  args: string[],
  options?: { cwd?: string; env?: Record<string, string> },
): Promise<{ stdout: string; stderr: string }> {
  return runCmd(cmd, args, options);
}

export async function execSilent(cmd: string, args: string[]): Promise<string | null> {
  return tryRunCmd(cmd, args);
}

export async function tmux(...args: string[]): Promise<string | null> {
  return execSilent("tmux", args);
}

export async function git(args: string[], cwd?: string): Promise<string | null> {
  try {
    const { stdout } = await exec("git", args, { cwd });
    return stdout;
  } catch {
    return null;
  }
}

export async function gh(args: string[]): Promise<string | null> {
  return execSilent("gh", args);
}

export async function getTmuxSessions(): Promise<string[]> {
  const output = await tmux("list-sessions", "-F", "#{session_name}");
  if (!output) return [];
  return output.split("\n").filter(Boolean);
}

export async function getTmuxActivity(session: string): Promise<number | null> {
  const output = await tmux("display-message", "-t", session, "-p", "#{session_activity}");
  if (!output) return null;
  const ts = parseInt(output, 10);
  return isNaN(ts) ? null : ts * 1000;
}
