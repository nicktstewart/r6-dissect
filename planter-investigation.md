# Codex notes: planter-signal investigation plan

Repo: `C:\Projects\r6-dissect`

Goal:
determine whether newer replay files contain a direct or indirect planter identity signal that is more reliable than the current timer/alive-player heuristics.

## Working assumption

Ubisoft can replay the same `.rec` file on another machine and still show the correct planter.
That strongly suggests the replay stream contains enough information to recover planter identity.

What is unknown:
- whether the planter is encoded directly in a packet near plant start/completion
- whether the signal is attached to another entity such as the defuser actor / gadget / interaction object
- whether the signal is only visible through a sequence of packets rather than one explicit field

## Investigation target

Start with one round from `test-files\match2` that:
- definitely contains a completed plant
- has the smallest amount of unrelated player traffic
- already parses reasonably well today

Best first target:
- `match2` round 2

Reason:
- 1v1 replay
- clean plant with no disable
- minimal noise compared with `match5`
- attacker identity is already known from game outcome and existing parser output

Secondary targets:
- `match2` round 3
- `match2` round 4
- `match3` corresponding rounds
- `match5` round 4 only after a candidate signal is found

## Deliverables

Produce:
1. a narrowed list of packet regions that correlate with plant start / plant complete
2. evidence for whether one of those regions carries a player identity or object ownership signal
3. a conclusion:
   - direct planter ID found
   - indirect but reliable planter signal found
   - no clear signal found from tested regions

If successful:
- propose a parser change using the new signal
- keep current fallback logic only as backup

## Constraints

- do not start by diffing entire replays byte-for-byte without event anchoring
- do not rely on kill proximity as a primary explanation
- prefer signals that are stable across `match2` and `match3`
- any candidate signal must be tested from both replay perspectives

## Phase 1: Build a focused replay corpus

For each of these rounds, isolate the raw replay chunk and its expected event window:

- `match2` round 2
  - completed plant
  - no disable
- `match2` round 3
  - completed plant
  - completed disable
- `match2` round 4
  - completed plant
  - planter dies before disable
- `match3` corresponding rounds
  - same underlying events from opposite perspective

Record for each:
- replay file path
- round number
- expected planter
- expected disabler if any
- approximate parser event times:
  - plant start
  - plant complete
  - disable start
  - disable complete

## Phase 2: Anchor the event windows in decompressed data

Use the existing decompressed replay stream and current packet listeners to locate the byte offsets of:
- defuser timer packets
- nearby time packets
- nearby match feedback packets
- nearby scoreboard packets
- any recurring packet signatures before and after plant events

Goal:
create a byte window around each plant event, for example:
- `N` bytes before timer packet
- timer packet
- `N` bytes after timer packet

Do this first on `match2` round 2.

Output per event:
- replay time
- parser event type
- absolute offset in decompressed stream
- hex dump around the offset

## Phase 3: Compare plant windows against non-plant windows

Within the same round and across rounds, compare:
- plant-start windows vs ordinary timer updates
- plant-complete windows vs ordinary timer updates
- plant-complete windows vs disable-complete windows

Look for fields that change only when:
- attack-side player is interacting with objective
- defuser becomes planted
- ownership of planted device changes from “carried” to “world object”

Candidate patterns:
- 4-byte values matching known player `DissectID`
- 8-byte values matching `uiID`
- profile IDs or partial GUID fragments
- repeated object IDs linked to the planter across rounds
- actor references that persist from start to completion

## Phase 4: Correlate packet bytes with known player identities

From `readPlayer` / operator swap parsing, collect all known identifiers for each player:
- username
- `DissectID`
- `uiID`
- profile ID
- team index
- operator

Then scan the plant windows for:
- exact matches
- little-endian / partial matches
- nearby reference chains where one packet mentions an object and another packet maps that object to a player

This should answer:
- does the plant packet directly name a player?
- does it mention a stable actor/object that can be linked to a player elsewhere?

## Phase 5: Entity/ownership hypothesis testing

If no direct player ID is visible, test these hypotheses:

1. Defuser carrier object
- there may be an actor/object representing the carried defuser
- a nearby packet may transfer that object into planted state
- ownership of that object may imply planter

2. Interaction actor
- a packet near timer updates may encode “current interacting actor”
- this may not be the timer packet itself

3. Animation/state replication
- there may be a player-state packet that changes only for the planter
- example: stance, interaction state, gadget state, objective action enum

4. Objective actor replication
- planted defuser object may spawn with a creator/instigator field
- creator may remain visible after plant completes

## Phase 6: Cross-perspective validation

For any candidate signal found in `match2`:
- verify the same signal exists in `match3`
- confirm it points to the same real-world player, not the replay owner
- confirm it survives cases where the planter later dies

Reject any signal that:
- flips meaning by replay perspective
- only works when one player is alive
- depends on kill order

## Phase 7: Stress with `match5`

Once a candidate works on `match2`:
- test it on `match5` round 4
- verify it picks `OreoSenpai` as planter
- verify disable attribution still lands on `ironsmithers`

This is the main falsification round because current fallback heuristics are weakest there.

## Phase 8: Parser integration strategy

If a direct planter signal is found:
- parse it at plant start if possible
- store it in dedicated planter state
- use completion only as confirmation

If only a completion-time signal is found:
- use it directly for `DefuserPlantComplete`
- optionally backfill `DefuserPlantStart` attribution if the same actor is plausible

If only an indirect ownership signal is found:
- add a resolver layer:
  - object ID -> player ID
  - player ID -> username

Retain current round-end reconciliation only as fallback.

## Concrete first commands to run

1. Generate decompressed data for `match2` round 2.
2. Find the defuser timer signature occurrences in that decompressed stream.
3. Log absolute offsets and surrounding bytes for:
   - timer packets before plant
   - plant start
   - plant complete
4. Compare those windows with known player identifiers from the same round.
5. Repeat the same exact process on the corresponding `match3` round.

## Expected outcomes

Best case:
- find a direct player/object reference near plant that can be parsed robustly

Medium case:
- find a stable indirect signal that can be linked to player identity using already parsed data

Worst case:
- confirm current replay samples do not expose a recoverable direct planter signal in the inspected regions, which would justify keeping heuristic fallback and focusing on stronger reconciliation

## Notes for implementation after analysis

If a new signal is found, prefer:
- small targeted parser additions
- explicit side validation
- replay-perspective validation tests
- regression tests for:
  - `match2`
  - `match3`
  - `match5`

## Phase 1 execution: `match2` round 2

Target replay chunk:
- `test-files\match2\Match-2026-03-08_19-55-32-26676-R02.rec`

Ground-truth expectation from current parsed output:
- planter: `Mori.CU`
- plant start: currently parsed as `0:44`
- plant complete: currently parsed as `2:26`
- no disable in this round

Decompressed dump generated:
- `test-files\match2\round2.dump`
- size: `8,383,383` bytes

Known players for this round:
- attacker: `Mori.CU`
- defender: `MoriRecruit`

## Phase 2 findings: anchored timer windows

Timer packet signature scanned:
- `22 A9 C8 58 D9`

Total occurrences in `round2.dump`:
- `195`

### Cluster A: empty-string timer packets near raw round time `44`

Representative offsets:
- `8131191`
- `8131292`
- `8132976`
- `8133104`

Nearby decoded context:
- previous time packet offset: `248`
- previous raw round time: `44`
- timer payload: empty string

Representative bytes:
```text
... 22-A9-C8-58-D9-00-22-51-B4-4C-48-08-FF-FF-FF-FF-FF-FF-FF-FF ...
```

Observation:
- same timer signature
- payload length byte is `00`
- does not look like the real plant-progress countdown

### Cluster B: real countdown packets near raw round time `153` to `146`

Representative first countdown offset:
- `8279348`

Representative first decoded values:
- `8279348` -> timer `6.968`, previous raw round time `153`
- `8279466` -> timer `6.931`, previous raw round time `153`
- `8279577` -> timer `6.895`, previous raw round time `153`

Representative terminal countdown offsets:
- `8296218` -> timer `0.405`, previous raw round time `146`
- `8296311` -> timer `0.368`, previous raw round time `146`
- `8296404` -> timer `0.332`, previous raw round time `146`
- `8297231` -> timer `0.007`, previous raw round time `146`

Representative bytes at countdown start:
```text
... 22-A9-C8-58-D9-05-36-2E-39-36-38 ...
```

Interpretation:
- payload length byte `05`
- ASCII timer string `"6.968"`
- this is the real plant-progress countdown stream

Observation:
- the countdown spans raw round times `153` down to `146`
- that corresponds to `2:33` down to `2:26` remaining
- this aligns with a plausible plant progress window

### Cluster C: late `0.001` packet under raw round time `44`

Representative offset:
- `8297445`

Decoded context:
- timer payload: `0.001`
- nearby time packet offsets:
  - `8297297` -> raw round time `44`
  - `8297601` -> raw round time `44`

Nearby timer offsets:
- `8296952` -> `0.114`
- `8297045` -> `0.080`
- `8297138` -> `0.044`
- `8297231` -> `0.007`
- `8297445` -> `0.001`

Representative bytes:
```text
... 22-A9-C8-58-D9-05-30-2E-30-30-31 ...
```

Observation:
- the terminal countdown value continues into a region whose nearest decoded round time is `44`
- this suggests the timer signature is being reused across multiple contexts or the nearby time association is not one-to-one

## Main takeaway from Phase 2

There are at least two distinct packet contexts using the same timer signature:

1. empty-payload timer packets near raw round time `44`
2. real decimal countdown packets near raw round time `153` to `146`

This strongly suggests the current parser is conflating:
- a non-countdown / state-change packet sequence
- the real plant-progress countdown sequence

Practical implication:
- the current parsed `DefuserPlantStart` time for this round (`0:44`) is probably anchored to the wrong timer context
- the countdown beginning near raw round time `153` is the better candidate for true plant-start anchoring

## Suggested next step

Proceed to Phase 3 and Phase 4 on this same round:
- compare countdown-start windows against countdown-end windows
- compare both against the empty-payload `44` cluster
- scan surrounding bytes for player-linked identifiers:
  - `DissectID`
  - `uiID`
  - other stable actor/object references

## Phase 3 findings: countdown windows vs false `44` windows

Compared windows:
- false `44` cluster:
  - `8131191`
  - `8131292`
  - `8132976`
  - `8133104`
- real countdown start:
  - `8279348`
  - `8279466`
  - `8279577`
- real countdown end:
  - `8296218`
  - `8296311`
  - `8296404`

### Stable pre-timer structure in real countdown packets

Representative pre-timer slices:

Countdown start:
```text
... 18-37-46-6C-04-BE-57-02-00-23-9E-3D-03-F0-00-00-00-00-E5-8C-06-E9-04-00-00-00-00-22-E9-A3-7F-EB-04-00-00-80-3F
... 18-37-46-6C-04-99-57-02-00-23-9E-3D-03-F0-00-00-00-00-E9-A3-7F-EB-04-51-A5-7E-3F
... 18-37-46-6C-04-75-57-02-00-23-02-3F-03-F0-00-00-00-00-B6-F4-74-A3-04-16-00-00-00-23-9E-3D-03-F0-00-00-00-00-E9-A3-7F-EB-04-40-4C-7D-3F
```

Countdown end:
```text
... 18-37-46-6C-04-17-3E-02-00-23-9E-3D-03-F0-00-00-00-00-E9-A3-7F-EB-04-C0-3F-6D-3D
... 18-37-46-6C-04-F2-3D-02-00-23-9E-3D-03-F0-00-00-00-00-E9-A3-7F-EB-04-00-99-57-3D
... 18-37-46-6C-04-CE-3D-02-00-23-9E-3D-03-F0-00-00-00-00-E9-A3-7F-EB-04-20-05-42-3D
```

False `44` cluster:
```text
... 23-A9-C9-03-F0-00-00-00-00-E5-8C-06-E9-04-02-00-00-00-22-8D-68-F8-55-01-00-22-E9-A3-7F-EB-04-00-00-00-00
... 23-A8-C9-03-F0-00-00-00-00-E5-8C-06-E9-04-02-00-00-00-22-8D-68-F8-55-01-00-22-E9-A3-7F-EB-04-00-00-00-00
```

### Structural difference

Real countdown packets consistently include these nearby values:
- `18-37-46-6C-04-..-..-02-00`
- `23-9E-3D-03-F0-00-00-00-00`
- `E9-A3-7F-EB-04-<float>`

False `44` packets instead show:
- `23-A9-C9-03-F0...` or `23-A8-C9-03-F0...`
- `22-8D-68-F8-55-01-00`
- `E9-A3-7F-EB-04-00-00-00-00`

Conclusion from structural comparison:
- the real countdown packets are a distinct packet family or packet state
- the false `44` cluster is not just the same packet at a different timer value
- the current parser is almost certainly treating the wrong packet family as plant start

## Phase 4 findings: direct player-identity scan

Known parser identifiers for this round:

- `Mori.CU`
  - `DissectID = 19cf02f0`
  - `uiID = 370549116822`
- `MoriRecruit`
  - `DissectID = 04cf02f0`
  - `uiID = 409795469793`

Exact byte scans performed for:
- both `DissectID` values
- both 8-byte little-endian `uiID` values

### Whole-file scan result

The identifiers exist in the replay dump globally, but not near the plant windows:
- `Mori.CU DissectID`: found elsewhere, not in plant regions
- `MoriRecruit DissectID`: found elsewhere, not in plant regions
- `Mori.CU uiID`: found elsewhere, not in plant regions
- `MoriRecruit uiID`: found elsewhere, not in plant regions

### Broad region scan result

Broad countdown region scanned:
- `[8271156, 8305637]`

Broad false-`44` region scanned:
- `[8122999, 8141296]`

Result:
- no occurrences of either player's `DissectID`
- no occurrences of either player's `uiID`

### Implication

For `match2` round 2, the real countdown packets do not appear to carry:
- direct player `DissectID`
- direct player `uiID`
- any immediately obvious profile-ID fragment

So based on this round alone:
- there is no evidence yet of a direct planter ID embedded right next to the timer countdown packets
- the better candidates are the stable non-player object-like fields that repeat around every real countdown packet

## Current best candidate fields for Phase 5

These should be treated as likely object / state references, not player references:
- `18-37-46-6C-04-..-..-02-00`
- `23-9E-3D-03-F0-00-00-00-00`
- `E9-A3-7F-EB-04-<float>`

Most interesting of the three:
- `23-9E-3D-03-F0-00-00-00-00`

Reason:
- it is present in both countdown-start and countdown-end windows
- it is absent from the false `44` windows sampled above
- it looks more like a stable object/entity reference than a changing timer payload

## Conclusion after Phase 4

What is now supported by evidence:
- the parser's current start-event anchor is likely wrong for this round
- the real plant-progress countdown is a distinct packet context
- no direct player identifier has yet been found in the immediate or broad countdown region

What remains plausible:
- the planter may be encoded indirectly via an object/entity reference adjacent to the countdown packets
- that object/entity may be linkable to a player in another packet family

## Recommended next step

Proceed to Phase 5:
- trace the stable object-like fields around the real countdown packets
- search the dump for those fields outside the timer region
- see whether any nearby packets map those object references back to a player-related identifier

## Phase 5 findings: player-linked object references

Searched the real countdown-adjacent fields from `match2` round 2 against the full dump, starting with:
- `23-9E-3D-03-F0-00-00-00-00`
- neighboring object-like references that appear in the same packet family

### `match2` round 2: `23-9E...` is countdown-specific, but not player-mappable by itself

Whole-dump scan result for:
- `23-9E-3D-03-F0-00-00-00-00`

Occurrences:
- total: `194`
- inside real countdown region: `192`
- outside real countdown region: `2`
- inside false `0:44` region: `0`

The only two non-countdown hits are:
- `8223829`
- `8224450`

Those both sit in the earlier empty-payload timer family and share the same local structure as the real countdown packets, including a sibling player-like field:
- `23-22-CF-02-F0-00-00-00-00`
- followed later by
- `23-99-3D-03-F0-00-00-00-00`

Implication:
- `23-9E...` still looks like a stable objective/entity reference
- but the better ownership candidate is the neighboring `23-22-CF-02-F0...` field, because that one can be tied back to a player block elsewhere in the dump

### `match2` round 2: `23-22-CF...` maps back to `Mori.CU`

In the player-parsing region for `Mori.CU`, the dump contains:
- username `Mori.CU`
- nearby object references:
  - `1B-22-CF-02-F0-00-00-00-00`
  - `23-22-CF-02-F0-00-00-00-00`

Representative region:
- username offset around `3331`
- `23-22-CF...` offset around `3598`

In the player-parsing region for `MoriRecruit`, the analogous references are different:
- `1B-09-CF-02-F0-00-00-00-00`
- `23-09-CF-02-F0-00-00-00-00`

Representative region:
- username offset around `9306`
- `23-09-CF...` offset around `9896`

Inside the real countdown packet family, `match2` round 2 repeatedly carries:
- `23-22-CF-02-F0-00-00-00-00`

Representative countdown hit:
- `8279753`

Interpretation:
- the real plant countdown is carrying an object/entity reference that also appears in `Mori.CU`'s player block
- the corresponding defender-linked object (`23-09-CF...`) does not appear in the countdown family sampled here
- this is the first replay-derived link from the countdown packets back to the known planter that does not depend on kill timing

### `match3` round 2: same structure, different IDs, same real player

The same underlying round from the opposite replay perspective does not reuse the exact `CF-02-F0` object IDs.
Instead, the player blocks expose a different but structurally similar set of player-linked object references.

For `Mori.CU`, the player block contains:
- `1B-3F-0A-04-F0-00-00-00-00`
- `23-44-0A-04-F0-00-00-00-00`
- `23-3F-0A-04-F0-00-00-00-00`

Representative `Mori.CU` region:
- username offset around `22142`
- `23-44-0A-04-F0...` offset `21457`

For `MoriRecruit`, the player block contains different references:
- `1B-48-0A-04-F0-00-00-00-00`
- `23-48-0A-04-F0-00-00-00-00`

Representative `MoriRecruit` region:
- username offset around `29422`

Inside the real countdown family for `match3` round 2, a `Mori.CU`-linked object appears again:
- `23-44-0A-04-F0-00-00-00-00`

Representative countdown hit:
- `8770488`

The false `0:44` family in `match3` uses different neighboring fields and does not show the same countdown structure.

Interpretation:
- the exact object IDs are replay-instance-specific
- but the countdown still carries a player-linked object reference that resolves to `Mori.CU`
- that makes the signal perspective-stable in structure even though the raw IDs differ between `match2` and `match3`

## Current conclusion after Phase 5

Supported by evidence:
- no direct planter `DissectID` or `uiID` has been found near the real countdown packets
- the real countdown packets do carry an indirect object/entity reference that can be tied back to a player block
- for both `match2` round 2 and `match3` round 2, that player-linked object resolves to `Mori.CU`

Working hypothesis:
- newer replay files expose one or more player-owned replicated object references in the player packets
- the real plant countdown packets carry one of those same player-owned references for the active planter
- resolving `countdown object ref -> player block object ref -> username` may be a stronger attribution path than the current alive-player heuristic

Constraint before parser integration:
- this still needs falsification on `match5` round 4
- especially to verify the same approach chooses `OreoSenpai` as planter without collapsing onto the defender/disabler path

## Phase 7 check: `match5` round 4

Target file actually present on disk:
- `test-files\match5\Match-2026-03-07_17-53-32-28328-R04.rec`
- decompressed dump already present as `test-files\match5\r04.dump`

Current parser output for this round still misattributes:
- `DefuserPlantComplete -> ironsmithers`
- `DefuserDisableComplete -> ironsmithers`

Ground truth remains:
- planter: `OreoSenpai`
- disabler: `ironsmithers`

### Player-linked references found in player blocks

Representative early player-block references:

`OreoSenpai`
- username offset around `21988`
- stable nearby player-linked object:
  - `23-F2-8A-18-F0-00-00-00-00`

`ironsmithers`
- username offset around `16683`
- stable nearby player-linked object:
  - `23-01-8B-18-F0-00-00-00-00`

### Real countdown family near plant window

Late-round timer cluster begins around:
- `70850066`

Representative real countdown packets:
- `70850066` -> timer `6.620`
- `70851168` -> timer `6.583`
- `70851663` -> timer `6.544`
- terminal region around `70872231` -> timer `0.002`

Recurring objective-like field across the countdown:
- `23-B0-69-24-F0-00-00-00-00`

### Strong planter evidence in the countdown region

At offset `70851748`, inside the real countdown family, there is a packet containing:
- `23-A4-8A-18-F0-00-00-00-00`
- immediately followed by the username `OreoSenpai`

Representative slice:
```text
... 23-B0-69-24-F0-00-00-00-00-E9-A3-7F-EB-04-7E-33-70-3F-22-A9-C8-58-D9-05-36-2E-35-34-34 ...
... 23-A4-8A-18-F0-00-00-00-00-DE-AE-CA-09-01-00-22-49-D8-60-4F-01-00-22-5B-E8-47-28-0A-4F-72-65-6F-53-65-6E-70-61-69 ...
```

This is stronger than the `match2` / `match3` result because:
- it appears directly inside the real decimal countdown window
- it names `OreoSenpai` explicitly
- it does so before disable completion and without using kill proximity

### Negative check for defender collapse

Within the same late countdown window:
- no `OreoSenpai`-independent `ironsmithers` player-block object (`23-01-8B-18-F0...`) was found
- nearby `ironsmithers` string hits in this region were part of UI / feed text, not the countdown packet family itself

Implication:
- this falsification round does **not** collapse onto the defender/disabler path
- the replay-derived signal in `match5` points to the correct planter (`OreoSenpai`), even though the current heuristic parser still reports `ironsmithers`

## Updated conclusion

The candidate signal now survives the main falsification round:
- `match2` round 2: indirect countdown-linked object resolves to `Mori.CU`
- `match3` round 2: same structure from opposite perspective resolves to `Mori.CU`
- `match5` round 4: real countdown region contains an explicit `OreoSenpai`-named packet adjacent to the timer stream, and does not show the defender's known player-block object in that same countdown family

This is now strong enough to justify parser integration work:
- first identify the correct real countdown family
- then prefer countdown-adjacent player/object signals over alive-player heuristics for planter attribution
