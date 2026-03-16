# Y11 Scoreboard Packet Investigation Note

Date: 2026-03-13

Scope:

- `test-files\match8` rounds 1-4
- `test-files\match9` rounds 2 and 4

Method:

- added Y11-only packet-window logging in [scoreboard.go](/C:/Projects/r6-dissect/dissect/scoreboard.go), gated by `R6_SCOREBOARD_DEBUG=1`
- logged:
  - player corpus from `readPlayer()`
  - absolute decompressed offsets
  - packet family
  - packet value
  - fixed hex window
  - nearby player-owned entity refs already known to the parser

## Main findings

1. `EC DA 4F 80` is not just a "score packet" on Y11.
It appears to be a generic scoreboard cell update marker.

2. Y11 score-family packets often serialize as repeated 18-byte cells:

```text
<entityRef(9)> EC DA 4F 80 04 <value_le32>
```

The player-linked field is the 9-byte entity ref immediately before the marker, not the old fixed 13-byte `DissectID` offset.

3. Y11 kill-family packets (`1C D2 B1 9D`) do not share the same local shape as score-family packets.
They contain additional row-local refs after the value, and those refs appear to be reused by later assist packets.

4. Y11 assist-family packets (`4D 73 7F 9E`) also do not share the score-family shape.
In the clearest cases they sit in the same local row/object-ref cluster as nearby kill packets.

5. The current `readScoreboardScore()` listener is almost certainly reading multiple scoreboard columns, not just score totals.
The repeated `EC DA 4F 80` cells for the same row include values like `2842`, `3`, and `865` within the same local block.

6. Kill and assist packets have at least two distinct Y11 shapes:
- an early-round bootstrap/UI shape using `22 ...`-prefixed locals and immediate `kill -> assist -> score` triplets
- a later, meaningful row-update shape using player-owned `23 .. F7 01 F0` refs plus scoreboard-local `23 .. 79 0D F0` refs

7. In the later row-update shape, the `79 0D F0` refs look like per-column locals inside a row block, not direct player ids.
The same row block can contain:
- kill cell
- assist cell
- one or more `EC DA 4F 80` cells
with the row still anchored by nearby `F7 01 F0` player-owned refs.

## Representative windows

### Score family

`match8` round 3, `MoriRecruit`, offset `14202142`:

```text
00 00 00 01 00
23 49 35 06 F0 00 00 00 00
[EC DA 4F 80] 04 2C 01 00 00
1B 72 35 06 F0 00 00 00 00
27 C0 8D CA ...
```

Observed value: `300`

Conclusion:

- the owning ref is immediately before the marker: `23 49 35 06 F0 ...`
- the next ref `1B 72 35 06 F0 ...` is likely an alias/object tied to the same player row

`match9` round 4, `massaman`, offsets `62375083` and `62375101`:

```text
23 C9 EC 00 F0 00 00 00 00
[EC DA 4F 80] 04 1A 0B 00 00
23 1C E8 01 F0 00 00 00 00
EC DA 4F 80 04 03 00 00 00
23 D1 F7 01 F0 00 00 00 00
EC DA 4F 80 04 61 03 00 00
23 28 F7 01 F0 00 00 00 00
DE AE CA 09 ...
```

Observed values in the same row block: `2842`, `3`, `865`

Conclusion:

- this is a row serializer, not a dedicated "score total" packet
- each `EC DA 4F 80` cell appears to belong to the ref immediately before it
- one of the adjacent refs is likely a scoreboard-local row/object id

### Kill family

`match9` round 2, `Arison_KURONURI`, offset `53207868`:

```text
23 82 F7 01 F0 00 00 00 00
[1C D2 B1 9D] 04 05 00 00 00
23 23 F7 01 F0 00 00 00 00
E7 88 F6 A5 04 04 00 00 00
22 91 5F DF 38 01 01
22 99 FC 61 D9 01 01
23 3F F7 01 F0 00 00 00 00
EA 5F 14 10 ...
23 22 F7 01 F0 00 00 ...
23 3A F7 01 F0 00 00 ...
```

Observed value: `5`

Conclusion:

- kill-family packets do not look like score-family packets
- they include multiple refs after the value
- at least one post-value ref moves with the credited player row more plausibly than the old fixed-offset id

`match9` round 4, `bird135`, offset `62326426`:

```text
23 82 F7 01 F0 00 00 00 00
[1C D2 B1 9D] 04 0B 00 00 00
23 FB 79 0D F0 00 00 00 00
77 CA 96 DE 04 06 00 00 00
22 40 0A C8 29 04 30 00 00 00
23 FA 79 0D F0 00 00 00 00
B6 F4 74 A3 04 60 00 00 00
23 C6 F7 01 F0 00 00 00 00
4D 73 7F 9E ...
```

Observed value: `11`

Conclusion:

- the `0D79xxF0` refs are not known player-owned refs
- those refs are strong candidates for scoreboard-local row/object refs
- the later assist packet reuses the same local-ref family

### Assist family

`match9` round 4, `massaman`, offset `62326490`:

```text
23 C6 F7 01 F0 00 00 00 00
[4D 73 7F 9E] 04 04 00 00 00
23 95 79 0D F0 00 00 00 00
B6 F4 74 A3 04 24 00 00 00
23 D2 F7 01 F0 00 00 00 00
E7 88 F6 A5 ...
```

Observed value: `4`

Conclusion:

- assist-family packets can reuse the same `0D79xxF0` local-ref family seen in the kill row
- this is the clearest evidence so far that kill/assist attribution needs a scoreboard-local resolver, not another generic heuristic

`match9` round 2, unresolved assist, offset `53208931`:

```text
23 C9 EC 00 F0 00 00 00 00
[4D 73 7F 9E] 04 01 00 00 00
23 03 F7 01 F0 00 00 00 00
AF 98 99 CA ...
```

Observed value: `1`

Conclusion:

- assist-family packets are still not fully explained
- some cases expose only one nearby player-owned ref plus unrelated packet tail data
- this family is still unresolved enough that production mapping should not assume a single fixed assist shape yet

## Kill/assist linkage refinement

The strongest late-round example is `match9` round 4:

```text
23 82 F7 01 F0 ... [1C D2 B1 9D] 04 0B 00 00 00
23 FB 79 0D F0 ... 23 FA 79 0D F0 ...
23 C6 F7 01 F0 ... 4D 73 7F 9E 04 04 00 00 00
23 95 79 0D F0 ... 23 D2 F7 01 F0 ...
```

Observed parser results:

- kill packet maps to `bird135`
- immediately following assist packet maps to `massaman`

Interpretation:

- `23 C6 F7 01 F0` is a real player-owned row anchor for `bird135`
- the adjacent `79 0D F0` refs are scoreboard-local column/object refs used inside that row block
- the assist packet is not reusing the kill packet's direct player id; it is reusing the same local row block with a different column-local ref

The strongest score-row anchor is `match9` round 4 around `massaman`:

```text
23 C9 EC 00 F0 ... [EC DA 4F 80] 04 1A 0B 00 00
23 1C E8 01 F0 ... EC DA 4F 80 04 03 00 00 00
23 D1 F7 01 F0 ... EC DA 4F 80 04 61 03 00 00
23 28 F7 01 F0 ... DE AE CA 09 ...
```

Interpretation:

- one row can carry multiple adjacent `EC DA 4F 80` cells
- more than one player-owned `F7` ref can appear in the same local row block
- the practical resolver likely needs:
  - a row anchor
  - then a column-local mapping inside that row

This suggests the next parser experiment should not try:

- "pick the nearest owned ref"

It should try:

- identify the row block boundaries
- anchor the row to the player via the score-cell sequence
- map kill/assist column refs within that anchored row

## Focused row-block note: `match9` round 4

Generated from:

- `go run . -d test-files\match9\Match-2026-03-13_18-53-47-25248-R04.rec`
- `R6_SCOREBOARD_DEBUG=1`
- [scoreboard_row_block_report.ps1](/C:/Projects/r6-dissect/scripts/scoreboard_row_block_report.ps1)

Representative late row blocks:

1. Offset `62326426`, kill packet:
   - block offsets: `62326426-62326490`
   - ordered cells:
     - `kill@62326426[00000000F001F782:quirkyUtensil]=11`
     - `assist@62326490[00000000F001F7C6:bird135]=4`
   - owned refs:
     - `00000000F001F782:quirkyUtensil`
     - `00000000F001F7C6:bird135`
   - local refs:
     - `00000000F001E816`
     - `00000000F00D79FB`
     - `00000000F00D79FA`
     - `00000000F00D7995`
     - `00000000F001F7D2`
     - `00000000F00D4743`
     - `00000000F001F72E`

2. Offset `62326490`, assist packet:
   - same block offsets: `62326426-62326490`
   - ordered cells:
     - `kill@62326426[00000000F001F782:quirkyUtensil]=11`
     - `assist@62326490[00000000F001F7C6:bird135]=4`
   - owned refs:
     - `00000000F001F782:quirkyUtensil`
     - `00000000F001F7C6:bird135`
   - local refs:
     - same `79 0D` family and the same `F001E816` / `F00D4743` locals

3. Nearby score anchor for the same late cluster:
   - offset `62327721`:
     - `EC DA 4F 80` value `1740` immediately after `23 82 F7 01 F0`
     - that owned ref resolves to `quirkyUtensil`
   - offset `62327757`:
     - `EC DA 4F 80` value `730` immediately after `23 C6 F7 01 F0`
     - that owned ref resolves to `bird135`
   - this is the cleanest nearby evidence that the late kill/assist block is followed by explicit score cells for the same player-owned refs

Conclusions from this round-4 block:

1. `23 .. 79 0D F0` behaves like a column-local family inside one row block, not a direct player identifier.
2. The repeated owned refs `23 .. F7 01 F0` are the stable player anchors.
3. The row contains at least two player-owned refs, so "nearest owned ref wins" is not safe.
4. The unresolved part is which local ref denotes row anchor versus kill column versus assist column.
5. The practical parser rewrite still needs an explicit row parser, not another local-window heuristic.

## Practical conclusion

Current evidence supports:

- score family:
  - player-linked field found
  - use the entity ref immediately before each `EC DA 4F 80` cell
  - likely requires treating the packet as a generic scoreboard row/cell stream, not just "score"

- kill family:
  - direct player identity field not yet proved
  - scoreboard-local row/object refs likely present

- assist family:
  - scoreboard-local row/object refs likely present in some cases
  - family still partially unresolved

## What to do next

1. Rewrite Y11 score parsing around repeated cell blocks keyed by the preceding entity ref.
2. Add a temporary scoreboard-local ref table for investigation only:
   - populate from kill rows
   - check whether adjacent assist rows reuse the same local refs
3. Split Y11 kill/assist handling into two shapes:
   - bootstrap/UI shape
   - late row-update shape
4. For the late row-update shape, try a row-block parser instead of single-packet resolution.
5. Do not widen username scanning or generic nearby-ref learning again.
6. After explicit score mapping is in place, re-check whether the remaining DBNO failures in `match9` are now purely in kill/assist row linkage or in `reconcileKillsWithScoreboard()`.
