# Y11S2 Format Overhaul

This guide is the working handoff for rebuilding Y11S2 replay event parsing in `r6-dissect`.

The current parser can recover the 10-player header for the supplied Y11S2 files, but it does not currently recover kill/death match feedback or non-zero round stats. Treat the Y11S2 event format as significantly changed until proven otherwise.

## Current Status

- Header player recovery is fixed for the supplied `Match-2026-07-06*` files.
- `matchFeedback` is still effectively unavailable through the current parser.
- Kills/deaths/assists/headshots are currently zero because `PlayerStats()` derives them from `MatchFeedback` and/or scoreboard rows that are not decoded correctly for these files.
- Raw decompressed data shows that feedback-like data still exists. This is a parser/layout problem, not proof that Ubisoft removed kill/death data.

## Current Follow-up Priorities

1. Decode Y11S2 death-only feedback events. These are the highest priority missing event type.
2. Validate defuser events on Y11S2. Existing defuser detection still emits plant/disable events in some supplied exports, but it may be coming from a separate timer/event path rather than the changed `matchFeedback` packet layout.
3. Everything else can wait unless it blocks the first two items:
   - kill headshot flag,
   - assists,
   - locate objective,
   - BattlEye/player leave/other text feedback,
   - operator swap,
   - scoreboard reconciliation.

Known raw markers in `Match-2026-07-06_20-13-55-24040-R01.rec` after `--dump`:

```text
match feedback marker  59 34 E5 8B 04  found 15 times
kill indicator         22 D9 13 3C BA  found 17 times
score marker           EC DA 4F 80     found 109 times
assist marker          4D 73 7F 9E     found 11 times
kill score marker      1C D2 B1 9D     found 17 times
player marker          22 07 94 9B DC  found 10 times
time marker            1F 07 EF C9     found 187 times
```

Important observation: some Y11S2 feedback packets contain readable player names near the legacy kill indicator. Example from R01 raw dump: a packet near feedback marker `59 34 E5 8B 04` and kill indicator `22 D9 13 3C BA` contains both `Ch4inon` and `Mori.CU`.

## Supplied Test Inputs

Use only these new Y11S2 folders for this investigation unless deliberately comparing with older known-good replays:

```text
test-files/Match-2026-07-06_20-13-55-24040
test-files/Match-2026-07-06_21-16-49-24040
test-files/Match-2026-07-06_21-24-22-24040
test-files/Match-2026-07-06_22-15-05-24040
```

The initial crash repro was:

```text
test-files/Match-2026-07-06_20-13-55-24040
```

## Lobby Players

These four match folders appear to use the same lobby. Start with these 10 player names and header IDs:

```text
Team 1
Ch4inon           id=1335985450534950057
H031PON           id=14354689025032041422
smoking_man_T.K   id=8522320926524422832
Kyou.TARO         id=343759959938117717
AziStyle          id=12045000812079382179

Team 0
Mori.CU           id=1122773924631696113
yoshiko.BME       id=8171861996953094175
shiro_the_neko    id=16288017802923519379
Otyakururu_       id=1505915156071510819
NotS.ujufdn.LV    id=3206601951107630078
```

Subagents should inspect data around every occurrence of all 10 names, not only apparent kill packets. Names appear in header/player packets, feedback packets, UI strings, and possible noise. False positives are expected.

## Repro Commands

Dump one decompressed round:

```powershell
go run . --dump test-files\Match-2026-07-06_20-13-55-24040\Match-2026-07-06_20-13-55-24040-R01.rec -o .codex-go-cache-test\r01.bin
```

Export current JSON for one round:

```powershell
go run . test-files\Match-2026-07-06_20-13-55-24040\Match-2026-07-06_20-13-55-24040-R01.rec -o .codex-go-cache-test\r01.json
```

Export one match folder:

```powershell
go run . test-files\Match-2026-07-06_20-13-55-24040 -o .codex-go-cache-test\match-20.json
```

Count raw marker positions in a dump:

```powershell
@'
from pathlib import Path
b = Path(".codex-go-cache-test/r01.bin").read_bytes()
patterns = {
    "match_feedback_old": bytes.fromhex("5934E58B04"),
    "score": bytes.fromhex("ECDA4F80"),
    "assist": bytes.fromhex("4D737F9E"),
    "kills": bytes.fromhex("1CD2B19D"),
    "kill_indicator": bytes.fromhex("22D9133CBA"),
    "player_marker": bytes.fromhex("2207949BDC"),
    "time_y8": bytes.fromhex("1F07EFC9"),
}
for name, pat in patterns.items():
    positions = []
    start = 0
    while True:
        i = b.find(pat, start)
        if i < 0:
            break
        positions.append(i)
        start = i + 1
    print(name, len(positions), positions[:20])
'@ | python -
```

Search all player-name windows:

```powershell
@'
from pathlib import Path
b = Path(".codex-go-cache-test/r01.bin").read_bytes()
players = [
    "Ch4inon", "H031PON", "smoking_man_T.K", "Kyou.TARO", "AziStyle",
    "Mori.CU", "yoshiko.BME", "shiro_the_neko", "Otyakururu_", "NotS.ujufdn.LV",
]
for name in players:
    raw = name.encode("utf-8")
    positions = []
    start = 0
    while True:
        i = b.find(raw, start)
        if i < 0:
            break
        positions.append(i)
        start = i + 1
    print(name, len(positions), positions[:50])
'@ | python -
```

Print readable windows around candidate offsets:

```powershell
@'
from pathlib import Path
b = Path(".codex-go-cache-test/r01.bin").read_bytes()
positions = [67506786, 67506830]
for p in positions:
    s = max(0, p - 96)
    e = min(len(b), p + 320)
    chunk = b[s:e]
    printable = "".join(chr(c) if 32 <= c < 127 else "." for c in chunk)
    print("\\n--- pos", p, "---")
    print(chunk.hex(" "))
    print(printable)
'@ | python -
```

## Current Parser Hypothesis

Old parser path:

- `reader.go` listens for `59 34 E5 8B 04` and calls `readMatchFeedback`.
- For `CodeVersion >= Y9S1Update3`, `readMatchFeedback` skips 38 bytes, reads a one-byte `size`, and if `size == 0`, expects kill indicator `22 D9 13 3C BA` immediately after.
- That assumption fails for Y11S2. In observed Y11S2 packets, the marker and kill indicator still exist, but there can be additional ref/activity blocks between the marker and kill indicator. Some kill-like packets show names shortly after the kill indicator.

Do not assume the old structure is only shifted by a fixed offset. The format may have changed semantically.

## Investigation Strategy

1. Build a corpus table per round.
   - For every `.rec`, dump decompressed bytes.
   - Count marker occurrences for feedback, kill indicator, score, assist, kill score, player marker, and time marker.
   - Record current exported `roundNumber`, teams, score delta, and player list.

2. Build player-name occurrence maps.
   - For each of the 10 lobby players, list all offsets where the UTF-8 name occurs.
   - For each occurrence, capture at least `[-128, +384]` bytes and a printable view.
   - Classify the window manually: header/player packet, feedback/event packet, UI/noise, unknown.

3. Identify event packet families by visual inspection.
   - Start with windows containing two player names close together.
   - Cross-reference nearby markers:
     - `59 34 E5 8B 04`
     - `22 D9 13 3C BA`
     - time marker `1F 07 EF C9`
     - scoreboard markers `EC DA 4F 80`, `4D 73 7F 9E`, `1C D2 B1 9D`
   - Look for stable fields before/after names: one-byte string length, ref IDs, boolean-like bytes, operator IDs, headshot flag candidates.

4. Decode kill/death with conservative validation.
   - A normal 5v5 round should usually have 5-9 kills.
   - Some rounds can have fewer due to objective win, surrender, disconnect, or incomplete replay.
   - Avoid outputting 20+ kills from duplicate UI/log echoes.
   - De-duplicate by victim per round unless raw evidence proves DBNO/revive or repeated death semantics.
   - Do not count same-team target pairs as kills unless there is explicit TK support.

5. Reconcile against scoreboard counters only after feedback decoding is understood.
   - Scoreboard kill counters can validate kill totals.
   - Scoreboard rows should not be the source of truth for player identity.
   - If scoreboard counters disagree with feedback, log the disagreement and inspect both packet families.

6. Implement in small parser layers.
   - Add Y11S2-specific decode helpers instead of widening old Y9 offsets.
   - Keep old parser behavior intact for older versions.
   - Add corpus-backed tests from the supplied files.
   - Tests should assert non-zero match feedback and plausible kill totals, not exact stats until manually verified.

## Subagent Work Plan

Future Codex sessions should spawn multiple read-only explorer subagents before coding. Suggested tasks:

### Subagent A: Player Name Windows

Inspect all occurrences of all 10 player names in all four match folders. Produce a table:

```text
round, player, offset, nearby markers, other player names in window, classification, notes
```

Prioritize windows with two different player names within 128 bytes.

### Subagent B: Feedback Packet Layout

Start from every `59 34 E5 8B 04` occurrence. For each:

- find nearest `22 D9 13 3C BA` within the next 512 bytes,
- list player names within the next 512 bytes,
- record bytes between marker and kill indicator,
- infer candidate packet header/ref fields.

### Subagent C: Scoreboard Packet Layout

Start from every scoreboard marker:

```text
EC DA 4F 80
4D 73 7F 9E
1C D2 B1 9D
```

Compare nearby player refs/names against final round stats and feedback packet candidates. Determine if counters can be used as a validation layer.

### Subagent D: Round-Level Plausibility

For each supplied round:

- estimate expected winner from score delta,
- count likely kill packets by marker/name proximity,
- flag rounds with too few or too many candidates,
- identify final-forfeit or objective-only rounds.

## Acceptance Criteria

Minimum acceptable parser outcome:

- For the four supplied match folders, `matchFeedback` is not null for rounds where raw event data exists.
- Kill/death stats are non-zero where candidate event packets show kills.
- Typical rounds do not exceed plausible 5v5 kill counts.
- Header players remain exactly 10 for every supplied replay.
- Existing non-Y11S2 tests still pass.

Preferred outcome:

- Kills include killer, target, time, and headshot where available.
- Death-only events are handled without fabricating a killer.
- Defuser plant/disable events remain correct or are separately flagged as unknown for Y11S2.
- Scoreboard counters reconcile with parsed kills, or discrepancies are logged for targeted follow-up.

## Current Progress Log

Append future findings below. Use dated entries and include command snippets or offsets whenever useful.

### 2026-07-07

- Fixed header final-player parsing separately: the final in-progress player block must be appended when the header terminates at `teamscore1`.
- Verified the supplied Y11S2 files have 10 header players after `NewReader`.
- Confirmed `matchFeedback` remains empty/null with the current event parser.
- Confirmed raw decompressed R01 contains old feedback markers, kill indicators, scoreboard markers, time markers, and readable player names near candidate event packets.
- Important R01 marker counts:
  - `59 34 E5 8B 04`: 15
  - `22 D9 13 3C BA`: 17
  - `EC DA 4F 80`: 109
  - `4D 73 7F 9E`: 11
  - `1C D2 B1 9D`: 17
- Candidate packet near offset `67506786` / kill indicator around `67506830` contains `Ch4inon` and `Mori.CU`, indicating kill/death data is present but not decoded.

### 2026-07-08

- Implemented a first Y11S2-specific match feedback decoder.
  - `readMatchFeedback` now routes `CodeVersion >= Y11S2` through a Y11S2 helper instead of the old Y9 fixed-skip layout.
  - Y11S2 kill packets are decoded from `22 D9 13 3C BA` payloads containing two length-prefixed lobby player names.
  - `reader.go` also registers a Y11S2 listener directly on `22 D9 13 3C BA`; `recordKillUpdate` de-duplicates duplicate feedback-marker/direct-indicator hits.
- Fixed a scanner-progress blocker exposed by Y11S2 files:
  - false-positive listener packets, especially `readPlayer`, can return EOF-like errors after large seeks;
  - `Reader.Read()` now treats errors accepted by `Ok(err)` as recoverable for that listener and continues processing later marker matches.
- Preserved Y11S2 header players during full reads:
  - `deriveTeamRoles()` previously removed players whose parsed operator ID was `0`;
  - the supplied Y11S2 files use operator ID `0` plus seasonal fallback names, so a full `Read()` could delete all 10 players before feedback lookup;
  - for `CodeVersion >= Y11S2`, players with operator ID `0` are retained.
- Added `TestY11S2MatchFeedbackFromReplayEvents`.
  - Asserts R01 keeps 10 players after `Read()`.
  - Asserts Y11S2 kill/death feedback is non-empty.
  - Asserts parsed kill count is plausible for 5v5 and kill pairs resolve to opposite teams.

Verification commands used:

```powershell
$env:GOCACHE='C:\Projects\r6-dissect\.codex-go-cache-test\gocache'
$env:GOMODCACHE='C:\Projects\r6-dissect\.codex-verify\gomod'
go test ./dissect -run Y11S2 -count=1
go run . test-files\Match-2026-07-06_20-13-55-24040 -o .codex-go-cache-test\match-20-after.json
go run . test-files\Match-2026-07-06_21-16-49-24040 -o .codex-go-cache-test\match-21-16-after.json
go run . test-files\Match-2026-07-06_21-24-22-24040 -o .codex-go-cache-test\match-21-24-after.json
go run . test-files\Match-2026-07-06_22-15-05-24040 -o .codex-go-cache-test\match-22-after.json
```

Supplied-folder export summary after the change:

```text
match-20-after.json     rounds=12 players=[10 x12] kills=[7,7,5,9,6,8,7,9,6,5,7,5]
match-21-16-after.json  rounds=1  players=[10]    kills=[5]
match-21-24-after.json  rounds=11 players=[10 x11] kills=[7,8,6,5,8,7,8,7,7,6,8]
match-22-after.json     rounds=12 players=[10 x12] kills=[9,8,7,7,7,7,7,8,6,6,6,5]
```

`matchFeedback` event status for the supplied Y11S2 exports:

- Working now:
  - `Kill`: recovered with killer, target, `Time`, and `TimeInSeconds`.
  - Some existing defuser detection still emits events in these exports:
    - `DefuserPlantStart`
    - `DefuserPlantComplete`
    - `DefuserDisableStart`
    - `DefuserDisableComplete`
- Not working yet / not decoded from the Y11S2 feedback packet format:
  - kill `Headshot` flag is not decoded by the new Y11S2 helper yet;
  - death-only events without a killer are not decoded yet;
  - assists are not decoded from feedback packets;
  - `LocateObjective`, `Battleye`, `PlayerLeave`, `Other`, and Y11S2-specific non-kill feedback messages are not decoded;
  - `OperatorSwap` was not observed/validated as part of this Y11S2 feedback pass;
  - scoreboard score/kill/assist counters are not yet reconciled with parsed Y11S2 feedback and should not be treated as the source of truth.

Important caveats:

- Current Y11S2 kill decoding is deliberately conservative: it requires two known lobby player names with one-byte length prefixes near the kill indicator and relies on `recordKillUpdate` to reject same-team pairs and duplicate target deaths.
- The parser now meets the minimum target of non-null/non-empty kill `matchFeedback` for supplied rounds with raw kill evidence, but exact stat fidelity still needs manual validation against VOD/scoreboard evidence.
- `go test ./dissect -run Y11S2 -count=1` passes.
- `go test ./dissect` passed during the broader verification run.
- `go test ./...` is still not a clean signal in this workspace:
  - `tmp` contains multiple standalone `main` packages with redeclared `main`;
  - `dissect/test` includes Ubisoft-fetch dependent operator tests that failed with `could not get operators from Ubisoft`.

Follow-up resume notes:

- Y11S2 recovered kill updates now explicitly set `headshot:false` until the real Y11S2 headshot flag is decoded.
  - This preserves current stats behavior without implying decoded headshot fidelity.
  - It also prevents output paths that dereference `Headshot` from panicking on Y11S2 recovered kills.
- `WriteExcel` now defensively treats nil kill `Headshot` pointers as false.
- Verification after this follow-up:

```powershell
$env:GOCACHE='C:\Projects\r6-dissect\.codex-verify\gocache'
$env:GOMODCACHE='C:\Projects\r6-dissect\.codex-verify\gomod'
go test ./dissect -run Y11S2 -count=1
go test ./dissect
go run . test-files\Match-2026-07-06_20-13-55-24040 -o .codex-go-cache-test\match-20-after-resume.json
go run . test-files\Match-2026-07-06_20-13-55-24040 -f excel -o .codex-go-cache-test\match-20-after-resume.xlsx
```

- Export validation for `match-20-after-resume.json`:
  - 12 rounds exported.
  - every round retained 10 players.
  - kill counts remained `[7,7,5,9,6,8,7,9,6,5,7,5]`.
  - every recovered kill serialized a `headshot` field.

### 2026-07-08 Death-only / Defuser Follow-up

Priority order recorded for the next parser work:

1. Death-only feedback events.
2. Defuser plant/disable events.
3. Other non-kill feedback can wait.

Death-only investigation:

- Scanned all 36 supplied Y11S2 round dumps under `.codex-go-cache-test/y11s2-dumps`.
- Every proper Y11S2 kill-indicator feedback packet with lobby-player strings contained two length-prefixed player names.
- Found zero proper one-name kill-indicator payloads in the supplied corpus.
- Extra single-name occurrences near kill indicators were UI/custom-entity text such as `DYNAMIC_ICON`, not length-prefixed feedback payloads.
- Implemented conservative death-only support anyway:
  - if a Y11S2 kill-indicator payload has exactly one length-prefixed known lobby player, record it as `Death`;
  - if the player is already represented as a kill target or an existing death-only event, do not duplicate it;
  - no supplied Y11S2 sample currently exercises this path.

Defuser status from regenerated Y11S2 exports:

```text
match-20-after-deathonly.json
  round 4:  PlantStart H031PON, PlantComplete H031PON, DisableStart NotS.ujufdn.LV, DisableComplete NotS.ujufdn.LV
  round 8:  PlantStart NotS.ujufdn.LV
  round 9:  PlantStart Mori.CU, PlantComplete Mori.CU
  round 12: PlantStart Otyakururu_

match-21-16-after-deathonly.json
  no defuser events

match-21-24-after-deathonly.json
  round 3: PlantStart Ch4inon
  round 9: PlantStart NotS.ujufdn.LV

match-22-after-deathonly.json
  round 8: PlantStart NotS.ujufdn.LV
  round 9: PlantComplete NotS.ujufdn.LV
```

Defuser caveat:

- These events appear to come from the existing defuser timer / round-end inference path, not from a decoded Y11S2 `matchFeedback` packet family.
- Treat defuser as partially working on the supplied corpus, but not fully proven for Y11S2 format fidelity.
- The supplied samples include sparse defuser activity, so this still needs more targeted replay evidence or manual VOD validation.

CalypsoCasino defuser ground truth from manual review:

- Match folder: `test-files/Match-2026-07-06_20-13-55-24040`.
- JSON round indexes are zero-based; replay filenames are one-based.
- Round 4 / JSON `roundNumber=3` / `R04`:
  - plant start and complete are by `Mori.CU`;
  - disable start and complete are by `H031PON`;
  - current parser misattributes plant to `H031PON` and disable to `NotS.ujufdn.LV`, who is already dead.
- Round 8 / JSON `roundNumber=7` / `R08`:
  - `AziStyle` starts the plant but does not complete it.
- Round 9 / JSON `roundNumber=8` / `R09`:
  - `smoking_man_T.K` starts the plant but does not complete it; current parser misses this;
  - `Kyou.TARO` then starts and completes the plant; current parser has a simple attribution error.
- Round 12 / JSON `roundNumber=11` / `R12`:
  - `H031PON` starts plant a few times but never completes the plant.
- ClubhouseY10 match folder: `test-files/Match-2026-07-06_22-15-05-24040`.
- Round 9 / JSON `roundNumber=8` / `R09`:
  - `Ch4inon` has defuser and starts plant;
  - `Ch4inon` dies;
  - `AziStyle` picks up defuser, starts plant, and completes plant;
  - previous parser output attributed the plant completion to `Ch4inon`.

Implementation notes for Y11S2 defuser attribution:

- The old Y10S4+ path used role/alive fallbacks for defuser timer packets. That is not reliable for Y11S2 because the parsed side roles can disagree with the actual defuser actor evidence.
- Y11S2 defuser timer actor attribution now prefers direct entity refs from the timer packet:
  - first checks the ref at `packetStart+36`;
  - then checks the ref at `packetStart+11`, which appears on later packets in some attempts.
- The direct ref owner map is built lazily from length-prefixed lobby player-name windows in the replay. A ref is accepted only when one player is the clear owner.
- For Y11S2, missing direct actor refs no longer fall back to role/alive guesses. The parser waits for a direct actor ref in a later timer packet.
- Plant/disable attribution is locked to the first accepted actor for a continuous timer attempt and only resets when the timer jumps back up by more than two seconds.
- If a new direct Y11S2 actor ref appears after the locked planter/disabler has died, the attempt actor lock is reset. This handles defuser pickup after an interrupted plant.
- For Y11S2 completions, a direct actor index is trusted even if the role fallback would disagree.

CalypsoCasino result after the direct-ref fix:

```text
match-20-after-defuser-directref.json
  round 4:  PlantStart Mori.CU, PlantComplete Mori.CU, DisableStart H031PON, DisableComplete H031PON
  round 8:  PlantStart AziStyle
  round 9:  PlantStart smoking_man_T.K, PlantStart Kyou.TARO, PlantComplete Kyou.TARO
  round 12: PlantStart H031PON, PlantStart H031PON, PlantStart H031PON

match-22-after-defuser-pickup.json
  round 9:  PlantStart Ch4inon, PlantStart AziStyle, PlantComplete AziStyle
```

Supplied-folder export summary after the defuser direct-ref change:

```text
match-20-after-defuser-directref.json     rounds=12 players=[10 x12] kills=[7,7,5,9,6,8,7,9,6,5,7,5] deathOnly=0
match-21-16-after-defuser-directref.json  rounds=1  players=[10]    kills=[5]                         deathOnly=0
match-21-24-after-defuser-directref.json  rounds=11 players=[10 x11] kills=[7,8,6,5,8,7,8,7,7,6,8]   deathOnly=0
match-22-after-defuser-directref.json     rounds=12 players=[10 x12] kills=[9,8,7,7,7,7,7,8,6,6,6,5] deathOnly=0
```

Verification after the defuser direct-ref change:

```powershell
$env:GOCACHE='C:\Projects\r6-dissect\.codex-verify\gocache'
$env:GOMODCACHE='C:\Projects\r6-dissect\.codex-verify\gomod'
go test ./dissect -run 'Y11S2|DeathOnly' -count=1
go test ./dissect
go run . test-files\Match-2026-07-06_20-13-55-24040 -o .codex-go-cache-test\match-20-after-defuser-directref.json
go run . test-files\Match-2026-07-06_21-16-49-24040 -o .codex-go-cache-test\match-21-16-after-defuser-directref.json
go run . test-files\Match-2026-07-06_21-24-22-24040 -o .codex-go-cache-test\match-21-24-after-defuser-directref.json
go run . test-files\Match-2026-07-06_22-15-05-24040 -o .codex-go-cache-test\match-22-after-defuser-directref.json
```

Verification after this follow-up:

```powershell
$env:GOCACHE='C:\Projects\r6-dissect\.codex-verify\gocache'
$env:GOMODCACHE='C:\Projects\r6-dissect\.codex-verify\gomod'
go test ./dissect -run 'Y11S2|DeathOnly' -count=1
go test ./dissect
go run . test-files\Match-2026-07-06_20-13-55-24040 -o .codex-go-cache-test\match-20-after-deathonly.json
go run . test-files\Match-2026-07-06_21-16-49-24040 -o .codex-go-cache-test\match-21-16-after-deathonly.json
go run . test-files\Match-2026-07-06_21-24-22-24040 -o .codex-go-cache-test\match-21-24-after-deathonly.json
go run . test-files\Match-2026-07-06_22-15-05-24040 -o .codex-go-cache-test\match-22-after-deathonly.json
```

### 2026-07-08 Clubhouse Defuser Pickup Follow-up

- Investigated `test-files/Match-2026-07-06_22-15-05-24040`, round 9 / JSON `roundNumber=8` / `R09` on `ClubHouseY10`.
- Manual ground truth:
  - `Ch4inon` has defuser and starts plant;
  - `Ch4inon` dies;
  - `AziStyle` picks up defuser, starts plant, and completes plant.
- Previous parser output:
  - `DefuserPlantStart: Ch4inon`;
  - `DefuserPlantComplete: Ch4inon`.
- Raw timer packet evidence:
  - early short plant attempt has direct actor refs for `Ch4inon`;
  - later plant timer clusters have direct actor refs for `AziStyle`;
  - the timer restarted from about `6.67` to `6.98`, which was too small for the prior `>2s` timer-jump reset rule.
- Fix:
  - when a new direct Y11S2 actor ref appears and the currently locked planter/disabler has died by the current timer time, reset the attempt actor lock;
  - this allows defuser pickup after an interrupted plant without relying on role/alive fallback guesses.
- Added regression coverage in `TestY11S2DefuserAttributionFromTimerRefs`:

```text
Clubhouse round 9:
  DefuserPlantStart Ch4inon
  DefuserPlantStart AziStyle
  DefuserPlantComplete AziStyle
```

- Verification:

```powershell
$env:GOCACHE='C:\Projects\r6-dissect\.codex-verify\gocache'
$env:GOMODCACHE='C:\Projects\r6-dissect\.codex-verify\gomod'
go run . test-files\Match-2026-07-06_22-15-05-24040 -o .codex-go-cache-test\match-22-after-defuser-pickup.json
go test ./dissect -run 'Y11S2|DeathOnly' -count=1
go test ./dissect
```
