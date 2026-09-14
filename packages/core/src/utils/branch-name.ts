/** Branch fallback used when no tracker supplies a branch name. */
export function getFallbackBranchName(issueId: string | undefined, sessionId: string): string {
  if (!issueId) return `session/${sessionId}`;
  const isBranchSafe = /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(issueId) && !issueId.includes("..");
  const slug = isBranchSafe
    ? issueId
    : issueId
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .slice(0, 60)
        .replace(/^-+|-+$/g, "");
  return `feat/${slug || sessionId}`;
}
