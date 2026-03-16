# DBNO Attribution Guide

## Purpose

This guide is for a Codex instance working only inside this repository.

The goal is to improve kill attribution around DBNO (Down But Not Out) situations using replay evidence and ground truth from real matches.

Desired behavior:

- If player A downs target T and player B confirms the kill, the official kill should be credited to A.
- If player A downs target T and T later bleeds out, the kill should still be credited to A if replay evidence supports that.
- Ordinary non-DBNO kills should remain unchanged.

This should be solved the same way defuser detection was improved in this repo:

- collect real replay cases
- compare replay output to known ground truth
- inspect raw replay-side evidence around the relevant moments
- prefer replay-local evidence over broad heuristics
- add a focused regression suite when the logic is stable

## Relevant Files In This Repo

- `dissect/feedback.go`
  - main kill parsing logic
  - current place to implement DBNO-related attribution behavior
- `dissect/stats.go`
  - round and match stat aggregation
  - only relevant if final behavior changes how kills should be counted
- `dissect/test/defuser_detection_test.go`
  - example of the kind of replay-driven regression suite that should eventually exist for DBNO
- `codex-docs/guide.md`
  - existing design notes
  - includes the desired DBNO behavior and a general replay-analysis workflow
- `test-files/`
  - local replay fixtures mentioned by the design notes

## Current State Of The Parser

The current parser logic for kills lives in `dissect/feedback.go`.

Important facts:

- Kills are parsed from match feedback packets.
- The parser already filters same-team kills.
- The parser already prevents some duplicate kill records on the same target.
- There is currently no full replay-derived DBNO model.
- The problem to solve is not "add a public DBNO event type" unless that becomes clearly necessary.
- The real goal is kill-credit correctness.

If you need a narrow target, prefer this:

- keep the public event as a normal `Kill`
- change the credited killer when replay evidence shows the kill belongs to the original downer
- optionally store extra metadata such as who confirmed the kill, but only if it helps debugging or later analysis

## Desired Outcome

For each DBNO scenario, the parser should answer:

- who caused the down
- whether the target was later confirmed or bled out
- who should receive the official kill

The likely final policy is:

- confirm by same player: kill stays with that player
- confirm by different player: kill is credited to the original downer
- bleed-out: kill is credited to the original downer

## Existing Notes In This Repo

`codex-docs/guide.md` already says:

- the target issue is "Fix DBNO finish kill credit"
- desired behavior is that when A downs T and B confirms T, the official kill goes to A
- this should not break ordinary non-DBNO kills

That file should be treated as the starting product requirement, not the full implementation plan.

## Working Assumption

There are probably at least three layers of possible evidence in the replay data:

1. Packet-local signals near the kill packet
2. Nearby event history for the same target
3. Scoreboard-side evidence such as kill count deltas

The best solution will likely combine them in that order:

- first prefer packet-local evidence
- then use same-target history as a fallback
- then use scoreboard evidence only if it is clearly reliable

## Prior Heuristic Approach

An earlier DBNO attempt in a sibling repo is still useful as reference material, even though it should not be copied directly as the final design here.

Useful ideas from that approach:

- classify likely finish-off packets using packet-local markers near kill parsing
- walk backward through prior same-target events to find the original downer
- if a later event is judged to be a confirm, rewrite credited kill ownership to the original downer
- optionally preserve the actual confirmer as debug-only or supplemental metadata

Useful caution from that approach:

- it relied heavily on heuristics
- it sometimes synthesized DBNO state when replay evidence was incomplete
- it used broad fallbacks such as time-based inference and duplicate-kill windows

That means it is useful as a source of hypotheses, not as a production blueprint.

The parts worth reusing conceptually are:

- finish detection as a first-pass classifier
- backward search over same-target history
- reassignment of confirm kills to the original downer

The parts to treat as temporary or suspect are:

- introducing a public `DBNO` event type by default
- broad time-only finish heuristics
- synthetic DBNO events that hide uncertainty
- fixed duplicate-kill windows without replay-local confirmation

## Practical Investigation Strategy

### 1. Build a small ground-truth corpus

Collect replay examples for each case below:

- A downs T and A confirms T
- A downs T and B confirms T
- A downs T and T bleeds out
- A downs T and T is revived
- A lands a normal kill with no DBNO
- crowded rounds where multiple nearby combat events could confuse attribution

For each example, record:

- round number
- expected credited killer
- actual confirmer, if any
- whether the target bled out
- replay perspective
- why the ground truth is trusted

If possible, use paired replay perspectives for the same round, as was done conceptually for defuser analysis.

### 2. Instrument `dissect/feedback.go`

Add temporary debug output around kill parsing in `dissect/feedback.go`.

Log at least:

- replay time
- parsed killer username
- parsed target username
- raw bytes near the parsed kill packet
- whether a second event appears later for the same target
- scoreboard kill totals before and after the event, if available

The point is to identify stable replay-side signatures for:

- initial down
- later confirm
- bleed-out resolution

### 3. Compare raw packet windows

For each known DBNO case, compare:

- the first packet window that seems related to the down
- the later packet window for the confirm or bleed-out
- nearby ordinary kill windows that should not be treated as DBNO

Look for:

- stable marker bytes
- flags that differ between clean kills and confirms
- object or actor references that carry the downer's identity
- target-state transitions that happen only for DBNO cases

### 4. Compare replay output against scoreboard truth

If the replay output credits the confirmer but the scoreboard says the downer got the kill, that is strong evidence the parser needs reassignment logic.

But be careful:

- scoreboard evidence is good for validation
- scoreboard evidence is not always the best primary attribution source

The defuser work in this repo improved when replay-local evidence became the main signal and scoreboard data stayed mostly supplemental.

## Candidate Logic Shape

This is a recommended direction, not a claim that it is already correct.

### Phase A: classify likely DBNO-related follow-up kills

When parsing a kill event, first decide whether it looks like:

- a clean kill
- a kill confirm on an already-downed target
- a final resolution after a bleed-out

This classification should ideally come from packet-local evidence, not just time-based guessing.

### Phase B: search for same-target prior state

If the event looks like a confirm or bleed-out resolution:

- look backward for earlier same-target evidence
- prefer an earlier event that identifies the original downer
- if a prior event exists and the current credited killer differs, rewrite the credited killer to the original downer

### Phase C: keep public output simple

Unless replay evidence forces something more complex:

- keep the final public event as `Kill`
- do not add a public DBNO event type by default
- only add metadata if it clearly helps debugging or downstream consumers

This is the safest way to fix attribution without changing too much of the parser's public behavior.

## Public Model Guidance

Prefer keeping Nick's public model simple unless replay evidence clearly requires more:

- keep official output as plain `Kill`
- reassign the credited killer when DBNO replay evidence supports it
- only add extra metadata such as confirmer identity if it materially helps debugging or downstream consumers

This keeps the user-visible behavior narrow while still allowing better attribution.

## What Not To Do First

Do not start by adding a large DBNO domain model.

Reasons:

- it increases surface area before the replay evidence is well understood
- it may lock the parser into a representation that is not actually needed
- the user-visible bug is specifically about who gets credited the kill

Also avoid starting with broad time-only heuristics such as:

- "two kills on the same target within N seconds means DBNO"
- "time zero implies DBNO confirm"

Those may be useful as temporary probes, but they are weak as a final rule unless replay analysis proves they are stable.

## What A Good Replay-Driven Test Suite Should Look Like

Use `dissect/test/defuser_detection_test.go` as the structural model, not the content model.

A future `DBNO` regression suite should:

- open specific replay fixtures
- inspect specific rounds
- assert credited killer for the target
- assert no false reassignment on ordinary kills
- assert behavior holds across multiple replay perspectives when possible

Suggested expectations:

- round with A-downs/B-confirms => final credited killer is A
- round with A-downs/A-confirms => final credited killer is A
- round with A-downs/bleed-out => final credited killer is A
- round with normal kill => credited killer remains unchanged

## Suggested Implementation Order

1. Inspect `codex-docs/guide.md` and `dissect/feedback.go`.
2. Enumerate candidate replay files under `test-files/`.
3. Pick 3 to 5 ground-truth DBNO rounds.
4. Add temporary logging around kill parsing.
5. Identify stable replay-local signals for down vs confirm vs bleed-out.
6. Implement the smallest kill-credit reassignment logic that matches those signals.
7. Add replay-based regression tests.
8. Remove temporary logging.

## Decision Standard

A solution is not done just because it "looks plausible" in code.

It should be considered complete only when:

- it matches ground truth on the chosen DBNO replay corpus
- it does not regress ordinary kill parsing
- it works from more than one replay perspective where possible
- it has focused regression tests in this repository

## Short Summary

The right path is to solve DBNO attribution the same way defuser detection was strengthened:

- start from real replay cases
- compare to trusted ground truth
- inspect raw replay evidence around the event
- implement small, defensible attribution rules
- lock them in with replay-based tests

The main product requirement is simple:

- confirmed kills and bleed-out kills should be credited to the player who caused the DBNO, not incorrectly to the confirmer or some later event.
