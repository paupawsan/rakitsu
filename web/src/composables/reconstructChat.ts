import type { AgentEvent } from '../types';
import type { ChatBlock } from '../types/chat';

// Rebuild a chat conversation from a session's event stream. Used when the
// user loads a past chat session in the Debug view — the live WebSocket is
// gone, but the jsonl event log has enough to replay what the operator saw.
//
// Two reconstruction paths:
//
//   Modern (CHAT_TURN_START/END): precise — the start event carries the user
//   prompt verbatim and the end event carries the final answer + interrupted
//   flag.
//
//   Legacy: older sessions don't have CHAT_TURN events. We fall back to
//   walking root-level turn boundaries:
//     - PIPELINE_START / AGENT_START at the top of a turn → PIPELINE_START
//       sometimes carries `query` (the text runner.Run was called with),
//       which is the user's prompt for that turn. AGENT_START doesn't carry
//       the query, so the user block is a placeholder.
//     - AGENT_END / EXECUTION_COMPLETE → final_answer for that turn's
//       assistant block.
//   The legacy path loses the user's prompt for single-agent configs; that's
//   a fundamental gap (the text was never emitted). We surface a placeholder
//   rather than hiding the turn.
export function reconstructChatBlocksFromEvents(events: AgentEvent[]): ChatBlock[] {
  // Prefer the modern path when any CHAT_TURN_START is present.
  const hasModern = events.some((e) => e.event_type === 'CHAT_TURN_START');
  if (hasModern) return reconstructModern(events);
  return reconstructLegacy(events);
}

function reconstructModern(events: AgentEvent[]): ChatBlock[] {
  const blocks: ChatBlock[] = [];
  for (const ev of events) {
    if (ev.event_type === 'CHAT_TURN_START') {
      const text = (ev.payload?.text as string) ?? '';
      blocks.push({ kind: 'user', text });
    } else if (ev.event_type === 'CHAT_TURN_END') {
      const final = (ev.payload?.final as string) ?? '';
      const interrupted = Boolean(ev.payload?.interrupted);
      const err = (ev.payload?.error as string) || '';
      blocks.push({
        kind: 'assistant',
        text: final || (err ? `Error: ${err}` : '(no response)'),
        interrupted,
      });
    } else if (ev.event_type === 'USER_INPUT_PENDING') {
      const q = (ev.payload?.question as string) ?? '';
      blocks.push({ kind: 'system', text: `Agent asked: ${q}` });
    } else if (ev.event_type === 'USER_INPUT_ANSWERED') {
      const last = blocks[blocks.length - 1];
      if (last && last.kind === 'system' && last.text.startsWith('Agent asked:')) {
        last.text += ' (answered)';
      }
    }
  }
  return blocks;
}

// Top-level root-event detection: an event is the start of a turn when it is
// a PIPELINE_START or an AGENT_START that is NOT nested inside another
// orchestration (iteration === 0 is a best-effort heuristic since the event
// stream lacks a clean "root" flag; good enough for chat runs where the
// runner starts fresh each turn).
function isRootStart(ev: AgentEvent): boolean {
  return ev.event_type === 'PIPELINE_START' || ev.event_type === 'AGENT_START';
}
function isRootEnd(ev: AgentEvent): boolean {
  return (
    ev.event_type === 'PIPELINE_END' ||
    ev.event_type === 'EXECUTION_COMPLETE' ||
    ev.event_type === 'AGENT_END'
  );
}

function reconstructLegacy(events: AgentEvent[]): ChatBlock[] {
  const blocks: ChatBlock[] = [];
  let inTurn = false;
  let turnIdx = 0;
  let finalFromEnd: string | null = null;
  let depth = 0; // nesting depth — only depth=1 starts/ends mark turn boundaries

  for (const ev of events) {
    // Track nesting via PIPELINE_START/END + AGENT_START/END pairs.
    if (isRootStart(ev)) {
      depth++;
      if (depth === 1) {
        // New turn root.
        turnIdx++;
        const query = (ev.payload?.query as string) || '';
        blocks.push({
          kind: 'user',
          text: query || `(turn ${turnIdx} — user prompt not recorded)`,
        });
        inTurn = true;
        finalFromEnd = null;
      }
    } else if (isRootEnd(ev)) {
      depth = Math.max(0, depth - 1);
      // Capture the final answer from whichever end-event surfaces it.
      const maybeFinal = (ev.payload?.final_answer as string)
        || (ev.payload?.result as string)
        || '';
      if (maybeFinal) finalFromEnd = maybeFinal;
      if (depth === 0 && inTurn) {
        blocks.push({
          kind: 'assistant',
          text: finalFromEnd || '(no response captured)',
        });
        inTurn = false;
        finalFromEnd = null;
      }
    }
  }

  // Unterminated turn (session ended mid-generation): still surface what we have.
  if (inTurn) {
    blocks.push({
      kind: 'assistant',
      text: finalFromEnd || '(interrupted — no final answer recorded)',
      interrupted: true,
    });
  }

  if (blocks.length === 0 && events.length > 0) {
    blocks.push({
      kind: 'system',
      text:
        'Chat transcript unavailable — this session has no reconstructible turn boundaries. ' +
        'The raw event stream is still visible in the debug tree.',
    });
  } else if (blocks.length > 0) {
    blocks.unshift({
      kind: 'system',
      text:
        'Legacy session — reconstructed from event boundaries. User prompts may show placeholders when the original text was not emitted.',
    });
  }
  return blocks;
}
