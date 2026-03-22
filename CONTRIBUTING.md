# Contributing to Agent Orchestrator

Thanks for your interest in contributing! This guide will get you set up.

## Prerequisites

- **Node.js** 22+ (`node -v`)
- **pnpm** (`corepack enable` to activate)
- **tmux** (for session runtime — `apt install tmux` / `brew install tmux`)

## Setup

```bash
# 1. Fork and clone
git clone https://github.com/<your-username>/agent-orchestrator.git
cd agent-orchestrator

# 2. Install dependencies
pnpm install

# 3. Build all packages
pnpm build

# 4. Verify tests pass
pnpm test
```

## Development

```bash
# Build all packages
pnpm build

# Run tests (excludes web UI)
pnpm test

# Lint
pnpm lint

# Dev server (web UI)
pnpm dev
```

## Making Changes

1. **Create a branch** from `main`:
   ```bash
   git checkout -b fix/issue-42-description
   ```

2. **Make your changes** — keep commits focused and atomic.

3. **Run tests and lint** before pushing:
   ```bash
   pnpm test && pnpm lint
   ```

4. **Push and open a PR:**
   ```bash
   git push -u origin fix/issue-42-description
   gh pr create --base main
   ```

## Commit Conventions

We use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix    | Use for                          |
| --------- | -------------------------------- |
| `feat:`   | New features                     |
| `fix:`    | Bug fixes                        |
| `docs:`   | Documentation changes            |
| `test:`   | Adding or updating tests         |
| `chore:`  | Tooling, config, CI              |
| `refactor:` | Code changes that aren't fixes or features |

**Examples:**
- `feat: add session age display to status output`
- `fix: return valid JSON from ao status --json`
- `docs: add CONTRIBUTING.md`

## PR Process

1. Open a PR against `main`
2. Fill in the description — reference the issue (`Closes #42`)
3. Wait for CI to pass
4. A maintainer will review and merge

## Project Structure

This is a pnpm monorepo. Key packages:

- `packages/core` — shared types, config, session management
- `packages/cli` — the `ao` CLI
- `packages/web` — web dashboard

## Questions?

Open an issue or hop into [Discord](https://discord.gg/composio).
