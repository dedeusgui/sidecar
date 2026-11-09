#!/usr/bin/env python3
"""Context sufficiency: what the agent did after Sidecar delivered evidence (harness-only, stdlib).

Independent implementation of the "next action" principle documented by CodeGraph
(docs/benchmarks/explore-sufficiency.md @ f72c9d2, MIT): the action that follows a delivery, in the same
thread, says whether the delivery was enough. No CodeGraph code is used.

Deliveries are (a) a UserPromptSubmit hook response that carried context and (b) a completed
`sidecar explore` call. For each delivery the first substantive later action in the same thread is
bucketed:

  explore_again        another explore call: the answer was insufficient
  read_returned        Read/shell read of a file the delivery (or an earlier one) shipped: allocation
  read_named           read of a file the delivery only named: recall, pointed but not shipped
  read_not_returned    read of a file never delivered: recall
  search               Grep/Glob/shell search: still hunting
  moved_on             edit, test, other command or the final answer: sufficient to advance
  errored              the delivery itself failed (explore exited with an error)

Rules: only a call issued in a LATER message than the delivery counts (parallel calls of the same
message are `concurrent`); bookkeeping (ToolSearch, TodoWrite) is stepped over; subagents are separate
threads and a delegation is judged by the subagent's first substantive call.

"moved_on" is NOT correctness. Combine with the oracle verdict; never report it alone as a win.

Session-level signals: redundant re-reads (same file read again with no edit in between), reads of a
delivered file after it was edited (stale context), and deliveries with no demonstrable effect (no
shipped file was read, edited or tested afterwards in any thread).
"""
import re

BOOKKEEPING = {"bookkeeping"}
EXPLORE_CMD = re.compile(r"(^|[\s/;&|(])sidecar\s+explore\b")
PATH_IN_CONTEXT = re.compile(r"\(([\w./@+\-]+\.[A-Za-z0-9]{1,8}):\d+")
EXPLORE_SECTION = re.compile(r"^\*\*`([^`]+)`\*\*", re.M)
EXPLORE_NAMED = re.compile(r"\(([\w./@+\-]+\.[A-Za-z0-9]{1,8}):\d+\)|^- \[[A-Z]+\] ([\w./@+\-]+\.[A-Za-z0-9]{1,8}) ", re.M)
READ_TARGET = re.compile(r"(?:cat|head|tail|less|nl|sed -n\s+'[^']*'|sed -n\s+\"[^\"]*\"|sed -n\s+\S+)\s+([^\s|;&>]+)")

BUCKETS = ("explore_again", "read_returned", "read_named", "read_not_returned", "search", "moved_on", "errored")


def normalize_path(p, root_prefixes=("/work/",)):
    p = (p or "").strip().strip("'\"")
    for prefix in root_prefixes:
        if p.startswith(prefix):
            p = p[len(prefix):]
    return p.lstrip("./") if p.startswith("./") else p


def paths_in_hook_context(text):
    return sorted({normalize_path(m) for m in PATH_IN_CONTEXT.findall(text or "")})


def paths_in_explore_output(text):
    """Returned = files with a source section; named = files only pointed at."""
    returned = {normalize_path(p) for p in EXPLORE_SECTION.findall(text or "")}
    named = set()
    for a, b in EXPLORE_NAMED.findall(text or ""):
        p = normalize_path(a or b)
        if p and p not in returned:
            named.add(p)
    return sorted(returned), sorted(named)


def event_path(ev):
    """File a read/edit event touches, or None. Shell reads are parsed from the command."""
    if ev.get("kind") in ("read", "edit") and not ev.get("cmd"):
        return normalize_path(ev.get("key"))
    cmd = ev.get("cmd") or ""
    if ev.get("kind") == "read" and cmd:
        m = READ_TARGET.search(cmd)
        if m:
            return normalize_path(m.group(1))
    return None


def _first_substantive(events, start, thread, after_message, threads_of_delegation):
    """First later event in `thread` (a delegation resolves to its subagent's first call)."""
    for ev in events[start:]:
        if ev.get("thread_id", "main") != thread:
            continue
        if ev.get("message_id") == after_message:
            continue  # concurrent: issued before the delivery existed
        if ev.get("kind") in BOOKKEEPING:
            continue
        if ev.get("kind") == "delegate":
            sub = ev.get("tool_use_id")
            first = next((e for e in events if e.get("thread_id") == sub and e.get("kind") not in BOOKKEEPING), None)
            return first if first is not None else ev
        return ev
    return None


def classify(events, deliveries):
    """Bucket each delivery by the next action. `events` are ordered tool events (lab.tool_events shape plus
    optional kinds explore/delegate/bookkeeping); `deliveries` are dicts with order, thread_id, message_id,
    returned (paths), named (paths), errored (bool)."""
    shipped = set()
    out = []
    for d in sorted(deliveries, key=lambda x: x["order"]):
        rec = {"order": d["order"], "source": d.get("source"), "thread_id": d.get("thread_id", "main")}
        if d.get("errored"):
            rec["bucket"] = "errored"
            out.append(rec)
            continue
        shipped.update(d.get("returned") or [])
        start = next((i for i, e in enumerate(events) if e["order"] > d["order"]), len(events))
        nxt = _first_substantive(events, start, d.get("thread_id", "main"), d.get("message_id"), {})
        concurrent = sum(1 for e in events if e.get("message_id") == d.get("message_id") and e["order"] != d["order"]
                         and e.get("thread_id", "main") == d.get("thread_id", "main") and e.get("kind") != "explore")
        rec["concurrent"] = concurrent
        if nxt is None:
            rec["bucket"] = "moved_on"  # the turn ended: the agent answered
            rec["next"] = "final_answer"
        else:
            kind, path = nxt.get("kind"), event_path(nxt)
            rec["next"] = kind
            if kind == "explore":
                rec["bucket"] = "explore_again"
            elif kind == "read" and path:
                if path in shipped:
                    rec["bucket"] = "read_returned"
                elif path in (d.get("named") or []):
                    rec["bucket"] = "read_named"
                else:
                    rec["bucket"] = "read_not_returned"
            elif kind in ("search",):
                rec["bucket"] = "search"
            elif kind == "read":
                rec["bucket"] = "read_not_returned"
            else:
                rec["bucket"] = "moved_on"
        out.append(rec)
    return out


def session_signals(events, deliveries):
    """Redundant re-reads, stale reads after an edit, and deliveries without demonstrable effect."""
    last_read, edited_since, redundant, stale_after_edit = {}, set(), 0, 0
    delivered = set()
    for d in deliveries:
        delivered.update(d.get("returned") or [])
    touched = set()
    for ev in events:
        path = event_path(ev)
        if not path:
            if ev.get("kind") == "test":
                touched.add("<test>")
            continue
        touched.add(path)
        if ev.get("kind") == "edit":
            edited_since.add(path)
            last_read.pop(path, None)
            continue
        if ev.get("kind") == "read":
            if path in last_read and path not in edited_since:
                redundant += 1
            if path in edited_since and path in delivered:
                stale_after_edit += 1
                edited_since.discard(path)
            last_read[path] = ev["order"]
    no_effect = 0
    for d in deliveries:
        ret = set(d.get("returned") or [])
        if ret and not d.get("errored") and not (ret & touched):
            no_effect += 1
    return {"redundant_rereads": redundant, "stale_reads_after_edit": stale_after_edit,
            "deliveries_without_effect": no_effect, "deliveries": len(deliveries)}


def summarize(classified):
    counts = {b: 0 for b in BUCKETS}
    for c in classified:
        counts[c["bucket"]] = counts.get(c["bucket"], 0) + 1
    counts["concurrent"] = sum(c.get("concurrent", 0) for c in classified)
    counts["total"] = len(classified)
    return counts


def estimate_tokens(chars):
    return None if chars is None else round(chars / 4)


def token_origins(turn):
    """Tokens by origin. Server counters are reported as measured; Sidecar payloads and tool results are
    estimated from characters (marked); missing sources stay None (unknown), never zero."""
    hook = turn.get("injected_chars")
    explore = turn.get("explore_chars")
    tool = turn.get("tool_result_chars")
    return {
        "measured": {k: turn.get(k) for k in ("input_uncached", "cache_read", "cache_create", "output")},
        "estimated_from_chars": {"sidecar_hook": estimate_tokens(hook), "sidecar_explore": estimate_tokens(explore),
                                  "tool_results": estimate_tokens(tool)},
    }
