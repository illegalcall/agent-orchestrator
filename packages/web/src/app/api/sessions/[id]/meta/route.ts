import { type NextRequest } from "next/server";
import { getServices } from "@/lib/services";
import { jsonWithCorrelation, getCorrelationId } from "@/lib/observability";
import type { ActivityState, SessionStatus } from "@composio/ao-core/types";

/**
 * Minimal session metadata for instant tab title rendering.
 *
 * Unlike GET /api/sessions/[id], this endpoint reads only from local storage
 * (no SCM API calls) and should respond in <50ms. It returns the subset of
 * fields the session detail page needs to build a descriptive tab title.
 */
export interface SessionMeta {
  id: string;
  status: SessionStatus;
  activity: ActivityState | null;
  branch: string | null;
  /** True when this session has the "orchestrator" role. */
  isOrchestrator: boolean;
  /** Basic PR info available from local metadata (no enrichment). */
  pr: { number: number; branch: string } | null;
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const correlationId = getCorrelationId(_request);
  try {
    const { id } = await params;
    const { sessionManager } = await getServices();

    const session = await sessionManager.get(id);
    if (!session) {
      return jsonWithCorrelation({ error: "Session not found" }, { status: 404 }, correlationId);
    }

    const isOrchestrator =
      session.metadata?.["role"] === "orchestrator" || session.id.endsWith("-orchestrator");

    const meta: SessionMeta = {
      id: session.id,
      status: session.status,
      activity: session.activity ?? null,
      branch: session.branch ?? null,
      isOrchestrator,
      pr: session.pr ? { number: session.pr.number, branch: session.pr.branch } : null,
    };

    return jsonWithCorrelation(meta, { status: 200 }, correlationId);
  } catch {
    return jsonWithCorrelation({ error: "Internal server error" }, { status: 500 }, correlationId);
  }
}
