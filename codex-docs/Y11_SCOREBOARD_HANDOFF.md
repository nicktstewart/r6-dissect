# Y11 Scoreboard Handoff

Repo: `C:\Projects\r6-dissect`

Goal:
repair Y11 scoreboard mapping so round-local scoreboard-derived kill totals are trustworthy enough to drive `reconcileKillsWithScoreboard()` again.

This is the most practical path to fixing Y11 DBNO confirmer-vs-downer attribution in chaotic replays such as `match9`.

## Current reliability baseline

As of the latest pass, scoreboard work is no longer the main reliability blocker.

Recent production hardening already landed:

- unknown operators no longer crash role resolution
- unknown operator serialization now has season-aware fallback handling:
  - `Y11S2` unknown -> `Dokkaebi`
  - `Y11S3` unknown -> `Y11S3NewDefender`
  - `Y11S4` unknown -> `Y11S4UnknownOperator`
- defuser completion attribution is now reliability-first:
  - keep replay-derived attribution when available
  - then use start-event / completion inference
  - final fallback is a deterministic player on the acting side
- `DefuserPlantComplete` and `DefuserDisableComplete` should no longer be emitted with an empty username under normal parser flow

Practical implication:

- scoreboard parsing should now be treated as optional accuracy work, not required stability work
- do not weaken current reliability fallbacks just to pursue better Y11 DBNO correction
- if future scoreboard work fails or remains partial, parser output should still remain usable

## Why this matters

The parser already has a scoreboard-based correction path in:

- `dissect/scoreboard.go`
- `dissect/time.go`

Intended flow:

1. parse raw kill feed into `MatchFeedback`
2. parse end-of-round scoreboard values
3. run `reconcileKillsWithScoreboard()`
4. reassign wrongly credited within-team kills when scoreboard truth disagrees

That logic is directionally correct for DBNO confirm cases, but it only works if scoreboard packets map reliably to the correct player.

Known Y11 failures in `match9`:

- round 2
  - current output: `hagegore -> Arison_KURONURI`
  - expected credited killer: `quirkyUtensil`
- round 4
  - current output: `bird135 -> massaman`
  - expected credited killer: `quirkyUtensil`

## Current state

The repo currently contains:

- legacy scoreboard parsing plus Y11 fallback logic in `dissect/scoreboard.go`
- scoreboard reconciliation in `dissect/time.go`
- player identity extraction in `dissect/player.go`
- ID lookup helpers in `dissect/utils.go`
- investigation-only Y11 scoreboard logging and row-block extraction in `dissect/scoreboard.go`

The debug path is gated by:

- `R6_SCOREBOARD_DEBUG=1`

What improved:

- Y11 score-family packets now resolve by the 9-byte entity ref immediately before `EC DA 4F 80`
- `readScoreboardScore()` now keeps the max resolved score-cell value per player on Y11 instead of letting smaller sibling cells overwrite it
- some Y11 score packets now resolve to real usernames instead of always `N/A`
- score-family packet structure is materially better understood
- late Y11 kill / assist packets can now be grouped into row blocks for investigation
- `reconcileKillsWithScoreboard()` now logs parsed-vs-scoreboard Y11 kill totals under `R6_SCOREBOARD_DEBUG=1`
- `reconcileKillsWithScoreboard()` now skips Y11 reconciliation when kill-count disagreement is clearly untrusted

What is still not solved:

- many Y11 scoreboard packets still do not map
- scoreboard kill packets are only partially resolved
- Y11 kill-family totals currently behave like cumulative match counters, not round-local truth
- scoreboard-derived kill totals are not yet trustworthy enough for DBNO reassignment
- kill / assist attribution is still too heuristic on Y11
- the current blocker is reliable per-player kill-row ownership, not just delta math
- kill attribution in chaotic DBNO rounds is still accuracy-sensitive even though the parser is now in a better reliability state overall

## Main conclusion

This is no longer just an "ID mismatch" problem.

The old Y11 scoreboard readers are structurally wrong.

Legacy assumptions like:

- score packet: after `Uint32()`, skip `13`, then read a 4-byte player id
- kill packet: after `Uint32()`, skip `30`, then read a 4-byte player id
- assist packet: similar fixed-offset idea

do not hold on Y11. Those offsets often land on marker bytes or packet-local refs instead of a real player identity field.

Examples seen in logs:

- `22ecda4f`
- `24ab08ff`

Those are not player ids.

## Main findings so far

1. `EC DA 4F 80` is not a dedicated "score total packet".
   It behaves like a generic scoreboard cell update marker.

2. Y11 score-family packets often serialize as repeated cells:

```text
<entityRef(9)> EC DA 4F 80 04 <value_le32>
```

In those packets, the best player-linked field found so far is the 9-byte entity ref immediately before `EC DA 4F 80`.
That is now the preferred production path for Y11 score ownership.

3. The current `readScoreboardScore()` listener is likely reading multiple scoreboard columns, not just score totals.
   In one local row block, repeated score-family cells carried values such as `2842`, `3`, and `865`.
   The current production mitigation is to retain the max resolved score-cell value per player on Y11.

4. Y11 kill-family packets (`1C D2 B1 9D`) do not share the same local shape as score-family packets.
   They contain additional row-local refs after the value.

5. Y11 assist-family packets (`4D 73 7F 9E`) also do not share the score-family shape.
   In the clearest cases they sit inside the same local row/object-ref cluster as nearby kill packets.

6. Kill / assist packets have at least two distinct Y11 shapes:
   - an early bootstrap / UI shape using mostly `22 ...` locals
   - a later meaningful row-update shape using player-owned refs like `23 .. F7 01 F0` plus scoreboard-local refs like `23 .. 79 0D F0`

7. The late kill / assist packets are row-block updates, not isolated packets.

8. The `79 0D F0` refs look like per-column locals inside a row block, not direct player ids.

9. A prior experiment that linked kill / assist row blocks to nearby score blocks was not safe enough and was removed from production attribution.

10. The current Y11 kill-family mapping is still not reliable enough to derive round deltas.
    On `match9` round 2, the parsed-vs-scoreboard diffs look cumulative rather than round-local:
    - `Arison_KURONURI parsed=0 scoreboard=5`
    - `quirkyUtensil parsed=2 scoreboard=6`
    - `hagegore parsed=2 scoreboard=0`
    - `max_abs_diff=5`

11. Because of that, `reconcileKillsWithScoreboard()` now has a Y11 trust gate.
    If any Y11 parsed-vs-scoreboard kill diff exceeds `1`, reconciliation is skipped instead of performing a bad mass reassignment.

## What the logs suggest

From `match8` and `match9`:

1. Some Y11 score packets carry the username bytes directly in the local packet window.
2. Some Y11 score packets carry player-owned entity refs that can be linked back to player packets.
3. Some Y11 kill packets do not use the same local shape as the score packets.
4. Late-round `match9` kill / assist packets can be grouped into coherent row blocks.
5. `match9` round 4 is the clearest row-block ground truth.
6. `match9` round 2 is thinner and more ambiguous.
7. Row identity and column identity are separate problems.
8. Raw Y11 kill counters should be treated as cumulative match state until proven otherwise.
9. The remaining task is not "use deltas instead of totals" in isolation.
   It is "first map the kill rows to the right player reliably enough that the deltas mean anything."

That means the next pass should focus on explicit kill-row ownership and baseline/peak tracking, not broader heuristics in `scoreboardPlayerIndex()`.

## Relevant files

Primary:

- `dissect/scoreboard.go`
- `dissect/time.go`
- `dissect/player.go`
- `dissect/utils.go`

Useful supporting code:

- `dissect/defuse.go`
- `dissect/reader.go`

Reference docs:

- `codex-docs/Y11_SCOREBOARD_PACKET_INVESTIGATION_NOTE.md`
- `codex-docs/testing-guide.md`
- `codex-docs/planter-investigation.md`

## Ground-truth targets

Start with:

- `test-files\match8`
  - small and clean
  - useful for isolating scoreboard packet families
- `test-files\match9`
  - chaotic
  - exposes the real DBNO attribution failure

Focus rounds:

- `match8` rounds 1-4
- `match9` round 2
- `match9` round 4

## Recommended next pass

Do not keep extending heuristic fallbacks in production code.

Also:

- keep scoreboard correction optional
- keep the current Y11 trust gate
- do not let scoreboard experiments regress the now-stable operator and defuser fallback paths

Instead:

1. build a focused scoreboard corpus for the target rounds
2. log absolute decompressed offsets, packet family, value, and fixed-width hex windows for Y11 packets
3. separate score, kill, and assist packet families instead of assuming they share one structure
4. identify row-block boundaries in the late Y11 kill / assist shape
5. anchor kill / assist rows to players via explicit repeated structure
6. determine which local-ref pattern represents row anchor vs kill column vs assist column
7. once kill-row ownership is credible, derive round-local deltas from row baselines / peaks instead of trusting raw cumulative totals
8. only then let Y11 scoreboard totals drive reconciliation again

## Practical implementation direction

Preferred order:

1. rewrite Y11 score parsing around repeated cell blocks keyed by the preceding entity ref
2. keep row-block extraction for late kill / assist packets as investigation scaffolding
3. determine whether the `79 0D F0` family is a stable per-column local-ref mapping
4. solve explicit Y11 kill-row ownership before changing reconciliation math again
5. derive round-local kill deltas from trustworthy per-player row baselines / peaks
6. once scoreboard kill totals are credible, re-check `reconcileKillsWithScoreboard()` in `dissect/time.go`

Strong constraints:

- do not keep broadening heuristic scans in production code
- do not assume score packets and kill packets use the same identifier
- do not treat partial score-family success as proof that kill packets are solved
- do not reintroduce generic nearby-ref learning
- do not remove the Y11 trust gate until kill-row ownership is trustworthy
- do not make scoreboard-driven correction mandatory for usable parser output
- remove temporary packet-debug logging before finalizing
- treat `match9` round 4 as the main falsification case for any kill / assist resolver

## Good next deliverable

Produce a short kill-row ownership note for `match9` round 4 containing:

1. one or two representative late row blocks
2. the owned refs in each block
3. the local refs in each block
4. the ordered cells in each block
5. a conclusion about which local-ref pattern appears to represent:
   - row anchor only
   - kill column
   - assist column
   - still unresolved

Then extend it with a round-delta note for `match9` round 2 showing whether the same row anchor can be followed across the round.

Only after that should the kill parser be rewritten.

## Validation target

Success looks like:

- Y11 scoreboard packets map to real players instead of mostly `N/A`
- `Scoreboard.Players[*].Kills` is credible on `match8`
- rerunning `match9` gives round-local scoreboard truth strong enough for `reconcileKillsWithScoreboard()`
- the known DBNO correction targets become fixable:
  - round 2 `Arison_KURONURI`
  - round 4 `massaman`

If scoreboard mapping becomes correct but those cases still fail, the next bug is in `dissect/time.go`, not `dissect/scoreboard.go`.
