package agent

import "regexp"

// TurnGuardConfig selects which interim post-turn truthfulness checks an
// agent applies before committing its final answer. All checks are opt-in;
// the zero value runs nothing and a nil *TurnGuard is a full no-op.
//
// These are "cheap interim" guards. Each one cross-checks the final answer
// against a hard per-turn execution fact (did invoke_config fire? did a
// memory write happen?), so a correct turn cannot trip them — the signal is
// ground truth, not another heuristic. They are deliberately NOT a general
// validators framework, which would own the general lifecycle-point /
// Allow-Deny-Kill / VALIDATOR_* schema. Keep statuses and events here plain
// and interim so they don't collide if a fuller validators framework lands
// later.
type TurnGuardConfig struct {
	// ForceDelegation rejects a turn that produced zero invoke_config calls.
	// Set only for the ChatHost meta-agent under force_delegation.
	ForceDelegation bool
	// CheckStorageClaims rejects a final answer that claims a memory write
	// ("I've stored that…", "saved as `node-id`") when no MEMORY_WRITE
	// happened that turn. Enable only when memory tools are
	// actually registered, so a non-memory claim cannot be misread.
	CheckStorageClaims bool
	// CheckReasoningSpiral is a raw-CoT-spiral check. DEFERRED — a
	// length/phrase heuristic here would be purely textual with no
	// ground-truth cross-check, so it carries real false-positive risk on
	// long legitimate answers; its proper home is the provider/format
	// layer (the reasoning-promotion path). This flag is a seam for a
	// future implementer; it is never wired and never consulted today.
	CheckReasoningSpiral bool
}

// turnSignals are the hard per-turn execution facts the guards cross-check a
// final answer against. The agent loop populates them as tool calls execute
// and resets them at the per-turn TurnResetter boundary.
type turnSignals struct {
	invokeConfigCalled bool // invoke_config ran without error at least once this turn
	memoryWriteCount   int  // successful memory_add / memory_link / memory_retire executions this turn
	spawnCalled        bool // spawn_agent ran without error at least once this turn (counts as delegation)
}

// TurnGuard applies the configured interim post-turn checks.
type TurnGuard struct{ cfg TurnGuardConfig }

// NewTurnGuard returns a guard for the given config.
func NewTurnGuard(cfg TurnGuardConfig) *TurnGuard { return &TurnGuard{cfg: cfg} }

// Markers prepended to a rejected final answer. Kept distinct from the
// "[REASONING-ONLY OUTPUT]" marker so the two failure modes stay separable.
const (
	markerNoDelegation = "[NO-DELEGATION]"
	markerUnverified   = "[UNVERIFIED CLAIM]"
)

// Plain interim terminal statuses. Deliberately NOT validator_* so they do
// not collide with a fuller validators framework's status enum if it lands.
const (
	statusNoDelegation = "no_delegation"
	statusUnverified   = "unverified_store_claim"
)

var (
	// First-person completed memory write with a demonstrative, possessive,
	// or explicit memory-noun object: "I've stored that", "we saved your
	// note", "I recorded the fact", "I've retired the old entry". The object
	// set excludes idioms ("I've saved you time") and generic file/document
	// objects ("I saved the draft"), keeping the match to
	// information-persistence claims. Verbs are memory-flavoured only (no
	// "wrote"/"committed", which read as file/git side effects). retired /
	// superseded cover the memory_retire and memory_add(supersedes=...) ops
	// (bi-temporal KG) — without them a fabricated retire/supersede claim
	// slips past this guard entirely.
	storageClaimFirstPerson = regexp.MustCompile(`(?i)\b(?:i|we)(?:'ve| have)?\s+(?:just\s+)?(?:stored|saved|persisted|recorded|noted|retired|superseded)\s+(?:(?:that|this|it|down)\b|your\s+\w+|the\s+(?:fact|note|entry|rule|memory|memories|preference|detail|info|information|item|record)s?\b)`)
	// Canonical opener: "Got it—I've stored…".
	storageClaimGotIt = regexp.MustCompile(`(?i)got\s*it[\s—,.\-]*i(?:'ve| have)?\s+(?:just\s+)?(?:stored|saved|persisted|recorded|retired|superseded)`)
	// High-signal "<verb> as `<id>`" with a backtick/quoted identifier —
	// the node-id confirmation shape, unambiguous on its own.
	storageClaimAsQuotedID = regexp.MustCompile("(?i)\\b(?:stored|saved|persisted|recorded)\\s+(?:it\\s+|that\\s+)?as\\s+[`\"'][^`\"']+[`\"']")
	// High-signal "<verb> as <bare-node-id>" where the id carries a ':' or
	// '-' (e.g. session-recordings-fact, project:foo) — distinguishes a real
	// id from prose like "saved as a draft".
	storageClaimAsBareID = regexp.MustCompile(`(?i)\b(?:stored|saved|persisted|recorded)\s+(?:it\s+|that\s+)?as\s+\w+[:\-]\w[\w:\-]*`)
	// retired/superseded confirmations name the node id directly rather than
	// via "as <id>" ("retired `old-id`", "superseded deploy-checklist-old").
	storageClaimRetireSupersedeQuotedID = regexp.MustCompile("(?i)\\b(?:retired|superseded)\\s+(?:it\\s+|that\\s+)?[`\"'][^`\"']+[`\"']")
	storageClaimRetireSupersedeBareID   = regexp.MustCompile(`(?i)\b(?:retired|superseded)\s+(?:it\s+|that\s+)?\w+[:\-]\w[\w:\-]*`)
	// Instructional / future / interrogative / non-memory (file, disk)
	// phrasing — never a fabricated memory write.
	storageClaimNonMemory = regexp.MustCompile(`(?i)(how\s+(?:do|to|can)\s+(?:i|you)\b|to\s+(?:save|store)\b|you\s+(?:can|could|should|may)\s+(?:save|store)\b|\b(?:file|files|disk|directory|folder|document)\b)`)
)

// Check inspects a settled final answer against the turn's execution signals.
// It returns the marker to prepend plus a human-readable reason when the turn
// must be rejected, or ("","") to accept. A nil guard always accepts.
func (g *TurnGuard) Check(finalAnswer string, sig turnSignals) (marker, reason string) {
	if g == nil {
		return "", ""
	}
	if g.cfg.ForceDelegation && !sig.invokeConfigCalled && !sig.spawnCalled {
		return markerNoDelegation, "force_delegation is set but the turn produced no invoke_config or spawn_agent call"
	}
	if g.cfg.CheckStorageClaims && sig.memoryWriteCount == 0 && claimsStorage(finalAnswer) {
		return markerUnverified, "final answer claims a memory write but no MEMORY_WRITE occurred this turn"
	}
	return "", ""
}

// turnGuardDirective returns the corrective instruction injected as a user
// message when the guard grants its single retry. Phrased as a concrete next
// action so a weak model can recover rather than re-explain.
func turnGuardDirective(marker string) string {
	switch marker {
	case markerNoDelegation:
		return "You did not delegate this turn. Every turn MUST delegate to the underlying system — answering from your own knowledge is not allowed. Call invoke_config now with the user's request (spawn_agent also counts when fan-out is needed)."
	case markerUnverified:
		return "You claimed to have stored or saved information, but no memory write happened this turn. Either call memory_add (or memory_link) now to actually persist it, or rewrite your answer so it does not claim a write occurred."
	default:
		return "Your previous answer failed a post-turn check. Review it and correct it."
	}
}

// statusFor maps a marker to its plain interim terminal status.
func statusFor(marker string) string {
	switch marker {
	case markerNoDelegation:
		return statusNoDelegation
	case markerUnverified:
		return statusUnverified
	default:
		return "success"
	}
}

// claimsStorage reports whether text asserts a completed memory write. It is
// biased toward false-negatives: a first-person completed-write claim, the
// canonical "got it—I've stored" opener, or a "<verb> as <id>" node-id shape,
// with instructional/future/non-memory phrasing suppressed unless an
// unambiguous quoted/bare node-id is present.
func claimsStorage(text string) bool {
	asID := storageClaimAsQuotedID.MatchString(text) || storageClaimAsBareID.MatchString(text) ||
		storageClaimRetireSupersedeQuotedID.MatchString(text) || storageClaimRetireSupersedeBareID.MatchString(text)
	hit := asID ||
		storageClaimFirstPerson.MatchString(text) ||
		storageClaimGotIt.MatchString(text)
	if !hit {
		return false
	}
	// A node-id confirmation is unambiguous even amid other prose, so it is
	// never suppressed. Otherwise, instructional/future/non-memory contexts
	// suppress the match.
	if !asID && storageClaimNonMemory.MatchString(text) {
		return false
	}
	return true
}
