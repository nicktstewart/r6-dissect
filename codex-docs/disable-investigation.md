# Codex notes: disable-completion investigation

Repo: `C:\Projects\r6-dissect`

Goal:
narrow replay-side evidence for `DefuserDisableComplete` to packet families unique to active disable, without broadening noisy heuristics in 5v5.

Current parser status:
- replay-derived alias ownership works as evidence in 2-player rounds
- plant completion behavior is preserved and currently stable on the local corpus
- disable completion now works on the current ground-truth set, but the evidence quality still varies by round type

## Scope of this pass

Focused files and data:
- `dissect/defuse.go`
- `dissect/reader.go`
- `test-files\match2\round3.dump`
- `test-files\match2\round4.dump`
- `test-files\match3\round3.dump`
- `test-files\match3\round4.dump`
- `test-files\match5\r04.dump`
- `tmp\match6-r03.dump`
- `tmp\match7-r03.dump`

Constraint carried forward:
- do not treat the timer-alias path as the final 5v5 solution
- use `match5` only as falsification after isolating a narrower disable-only family

## High-level result

The active disable countdown is not just the idle planted countdown continued with a different timer value.
There is a specific transition packet family at the moment the countdown flips from idle to active disable.

In 1v1-style rounds, that transition packet carries:
- the objective ref
- the disable alias/object ref
- one defender-owned player ref immediately after the timer

That defender-owned ref is the cleanest new identity hint found in this pass.

However:
- the same broad region in `match5` also carries unrelated attacker/player identity packets
- a whole-window scan still collapses in crowded rounds
- this means the useful target is the transition packet shape, not the entire disable countdown window

## Anchored disable windows

### `match2` round 3

Known active disable tail:
- offsets `7210217` through `7226921`

Representative timers:
- `7210217 -> 6.922`
- `7226642 -> 0.159`
- `7226828 -> 0.058`
- `7226921 -> 0.024`

Stable refs in the active disable window:
- objective ref: `23 47 D5 02 F0 00 00 00 00`
- disable alias ref: `23 8A B1 04 F0 00 00 00 00`

Preceding idle countdown window:
- roughly `7181711` through `7199815`

Stable refs in the idle planted window:
- objective ref: `23 47 D5 02 F0 00 00 00 00`
- idle alias ref: `23 8B B1 04 F0 00 00 00 00`

### `match2` round 4

Two late timer clusters:
- plant countdown: `5871229` through `5889483`
- disable countdown: `5905352` through `5923378`

Disable-side stable refs:
- objective ref: `23 47 D5 02 F0 00 00 00 00`
- disable alias ref: `23 ED 1A 04 F0 00 00 00 00`

Plant-side stable refs:
- objective ref: `23 47 D5 02 F0 00 00 00 00`
- plant alias ref: `23 F2 1A 04 F0 00 00 00 00`

### `match3` round 3

Known active disable tail:
- offsets `7237945` through `7256659`

Representative timers:
- `7237945 -> 6.978`
- `7256360 -> 0.100`
- `7256463 -> 0.064`
- `7256659 -> 0.000`

Stable refs in the active disable window:
- objective ref: `23 56 0E 04 F0 00 00 00 00`
- disable alias ref: `23 DF 9D 06 F0 00 00 00 00`

Preceding idle countdown window:
- roughly `7208784` through `7227442`

Stable refs in the idle planted window:
- objective ref: `23 56 0E 04 F0 00 00 00 00`
- idle alias ref: `23 DE 9D 06 F0 00 00 00 00`

### `match3` round 4

Two late timer clusters:
- plant countdown: `5905721` through `5924586`
- disable countdown: `5940381` through `5959177`

Disable-side stable refs:
- objective ref: `23 56 0E 04 F0 00 00 00 00`
- disable alias ref: `23 7B 2D 06 F0 00 00 00 00`

Plant-side stable refs:
- objective ref: `23 56 0E 04 F0 00 00 00 00`
- plant alias ref: `23 7A 2D 06 F0 00 00 00 00`

## Differential finding: idle -> active disable transition packet

The first real disable packet in all four 1v1 windows has a narrower structure than the rest of the countdown.

### `match2` round 3 disable start packet

Offset:
- `7210217`

Relevant structure:
```text
... 23 3B BE 04 F0 ...
... 23 FD CE 02 F0 ... 66 AC 77 24 ...
... 22 6F D5 28 EB ...
... 22 51 18 B3 27 ...
... 22 EA 5F 14 10 ...
... 23 8A B1 04 F0 ... E5 8C 06 E9 04 01 00 00 00 ...
... 22 A9 C8 58 D9 05 36 2E 39 32 32 ...
... 23 09 CF 02 F0 ...
```

Important distinction from the idle terminal packet:
- idle terminal packet uses alias `23 8B B1 ...`
- disable start packet uses alias `23 8A B1 ...`
- disable start packet carries defender-owned ref `23 09 CF 02 F0 ...` immediately after the timer

### `match2` round 4 disable start packet

Offset:
- `5905352`

Relevant structure:
```text
... 23 0D 29 04 F0 ... B6 F4 74 A3 ...
... 23 FD CE 02 F0 ... 66 AC 77 24 ...
... 22 6F D5 28 EB ...
... 22 51 18 B3 27 ...
... 22 EA 5F 14 10 ...
... 23 ED 1A 04 F0 ... E5 8C 06 E9 04 01 00 00 00 ...
... 22 A9 C8 58 D9 05 36 2E 39 33 33 ...
... 23 09 CF 02 F0 ...
```

### `match3` round 3 disable start packet

Offset:
- `7237945`

Relevant structure:
```text
... 23 5C 0A 04 F0 ... 66 AC 77 24 ...
... 22 6F D5 28 EB ...
... 22 EB F6 16 50 ...
... 22 51 18 B3 27 ...
... 22 EA 5F 14 10 ...
... 23 DF 9D 06 F0 ... E5 8C 06 E9 04 01 00 00 00 ...
... 22 A9 C8 58 D9 05 36 2E 39 37 38 ...
... 23 48 0A 04 F0 ...
```

Important distinction from the idle terminal packet:
- idle terminal packet uses alias `23 DE 9D ...`
- disable start packet uses alias `23 DF 9D ...`
- disable start packet carries defender-owned ref `23 48 0A 04 F0 ...` immediately after the timer

### `match3` round 4 disable start packet

Offset:
- `5940381`

Relevant structure:
```text
... 23 5C 0A 04 F0 ... 66 AC 77 24 ...
... 22 6F D5 28 EB ...
... 22 EB F6 16 50 ...
... 22 51 18 B3 27 ...
... 22 EA 5F 14 10 ...
... 23 7B 2D 06 F0 ... E5 8C 06 E9 04 01 00 00 00 ...
... 22 A9 C8 58 D9 05 36 2E 39 37 31 ...
... 23 48 0A 04 F0 ...
```

## What seems real

Across both perspectives:
- the active disable family starts with a transition packet that differs from the idle planted terminal packet
- the transition packet carries a defender-owned ref right after the timer in all 1v1 disable cases inspected
- that defender-owned ref matches the known defender player block:
  - `match2`: `23 09 CF 02 F0 ...` for `MoriRecruit`
  - `match3`: `23 48 0A 04 F0 ...` for `MoriRecruit`

This is narrower and stronger than the earlier whole-window alias scan.

## Why this still does not solve 5v5

`match5` round 4 falsification still shows heavy contamination inside the later countdown cluster.

Late countdown clusters in `r04.dump`:
- earlier cluster: `70723632` through `70784473`
- later cluster: `70848758` through `70872231`

The later cluster contains:
- stable objective ref `23 B0 69 24 F0 ...`
- unrelated player refs from multiple players
- explicit attacker username packets such as:
  - `Wiz.ETS` near `70850120`
  - `OreoSenpai` near `70851720`
- location-name packets such as:
  - `2F Eastern Stairs`
  - `1F Lounge`

Representative slices:

At `70848758`:
```text
... 23 B0 69 24 F0 ... 22 A9 C8 58 D9 05 36 2E 39 37 35 ...
... 23 00 8B 18 F0 ...
... 23 47 B7 18 F0 ...
```

At `70850120`:
```text
... 22 A9 C8 58 D9 05 36 2E 36 32 30 ...
... 23 FE 8A 18 F0 ...
... 23 E0 8A 18 F0 ... 07 57 69 7A 2E 45 54 53
```

At `70851720`:
```text
... 22 A9 C8 58 D9 05 36 2E 35 34 34 ...
... 23 A4 8A 18 F0 ... 0A 4F 72 65 6F 53 65 6E 70 61 69 ...
```

Implication:
- a broad scan across the entire countdown region can still attach to the wrong player
- this explains the earlier collapse onto `xenialKnife`
- the countdown region in 5v5 contains enough attacker-side identity traffic that whole-window entity ownership is not discriminative

## Current best interpretation

The useful replay-side target is not:
- a global nearby username search
- a broad entire-window owned-ref vote

The useful target is:
- the first active-disable transition packet
- or a very small packet family immediately adjacent to it

That family has these characteristics in 1v1:
- objective ref
- disable alias ref
- state byte group `E5 8C 06 E9 04 01 00 00 00`
- timer string
- defender-owned ref immediately after the timer

But it is not yet proven that the same post-timer defender ref remains uniquely meaningful in 5v5.

## Additional validation: `match6` and `match7`

These two rounds were inspected after the initial `match2` / `match3` / `match5` investigation to check whether the raw replay shape still makes sense in unrelated chaotic rounds.

### `match6` round 3

Parsed result:
- `DefuserPlantComplete = Daddy.Mozzie`
- no `DefuserDisableComplete`
- attackers win after plant on kills, not on timer and not on disable

Raw-dump takeaways:
- `Daddy.Mozzie` appears in the familiar timer-adjacent player-tagged family around the plant window
- the round shows normal post-plant timer activity after completion
- there is no clear active-disable transition/countdown family later in the round

Interpretation:
- this round looks like a clean confirmation that the plant-complete path is real
- it also confirms the parser is not spuriously inventing a disable completion just because the defuser was planted

### `match7` round 3

Parsed result:
- `DefuserPlantComplete = PercSpinner`
- `DefuserDisableComplete = LundyTrades`

Relevant round events:
- `PercSpinner` plants at `1:54`
- `LundyTrades` kills `PercSpinner` at `0:26`
- `OO1..` kills `skipalangdang` at `0:23`
- `LundyTrades` starts disable at `0:18`
- `LundyTrades` completes disable at `0:11`

Raw-dump takeaways:
- `PercSpinner` appears in the same style of planter-side timer-adjacent family seen in the earlier plant-complete rounds
- `skipalangdang` and `PercSpinner` both appear in the planted/idle family before active disable
- `LundyTrades` appears in a defender-side timer-adjacent family during the active disable countdown
- the active disable region contains a locally consistent defender-linked sequence instead of the broad identity pollution seen in `match5`

Interpretation:
- `match7` supports the earlier conclusion that disable completion works when the active disable packet family is locally discriminative
- this looks less like a lucky heuristic hit and more like genuine replay-side disable evidence
- compared with `match5`, the key difference is not that the round is less chaotic overall, but that the active disable window is cleaner and more defender-specific

## Updated interpretation

The current local corpus supports three separate statements:
- plant completion is backed by a stronger and more stable replay family than disable completion
- disable completion can be replay-derived and reliable in chaotic rounds when the active disable window is locally discriminative
- the real failure mode is still polluted objective traffic, where the later countdown region contains enough unrelated attacker and defender identity packets to make whole-window scans collapse

This means the previous `match2` / `match3` finding still holds:
- the best disable target is the active-disable transition packet or an immediately adjacent micro-family

And `match6` / `match7` refine it:
- `match6` shows the parser can avoid creating a false disable completion when there is only post-plant combat
- `match7` shows the parser can still succeed in a crowded round if the disable window itself contains a specific defender-linked interaction sequence

## Recommended next step

Do not change parser behavior for large rounds yet.

Next research step:
1. Isolate only the first active-disable transition packet in `match2` round 3/4 and `match3` round 3/4.
2. Build a field-by-field diff of that packet against:
   - the immediately preceding idle `0.000` packet
   - the next disable countdown packet
3. Run the exact same field extraction on the later `match5` countdown cluster.
4. Determine which fields in that single transition packet are:
   - defender-linked in 1v1
   - absent from the plant cluster
   - absent from attacker username/location subfamilies in `match5`

If a field survives that comparison, it is a much better candidate than the current whole-window alias ownership scan.
