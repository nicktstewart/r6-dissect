# Codex CLI notes for `r6-dissect`

Assumptions:
- Repo: `C:\Projects\r6-dissect`
- Branch: `main` of your fork
- Codex can read the local repo and upstream PR diffs
- Goal:
  1. add Solid Snake
  2. add `FortressY10` from PR #126
  3. fix defuser detection
  4. fix DBNO finish kill credit
  5. when parsing a match folder, ignore a final round that ends by prep-phase forfeit

## What is already true
- `Rauora` and `Denari` are already on current `main`; do not re-add them.
- No new map needs adding other than `FortressY10` from PR #126.
- New operator to add: `SolidSnake`.

## Read only these first
1. `dissect/header.go`
2. `dissect/version.go`
3. `dissect/feedback.go`
4. `dissect/defuse.go`
5. `dissect/scoreboard.go`
6. `dissect/player.go`
7. `dissect/stats.go`
8. `dissect/test/operators_missing_test.go`
9. `dissect/test/reader_test.go`

## External refs worth checking
Only inspect these diffs/forks.

- PR #126 / `ImAAhmad` fork
  - use for:
    - `FortressY10`
    - DBNO / kill reassignment ideas
    - newer scoreboard handling
    - newer defuser handling

- PR #124 / `DarHa1531212` fork
  - use for:
    - smaller defuser-detection idea
    - timer-direction logic

## Suggested git setup
```bash
git remote add upstream https://github.com/redraskal/r6-dissect.git 2>/dev/null || true
git remote add pr126 https://github.com/ImAAhmad/r6-dissect.git 2>/dev/null || true
git remote add pr124 https://github.com/DarHa1531212/r6-dissect.git 2>/dev/null || true
git fetch --all --tags
git checkout -b fix/solid-snake-defuse-dbno-forfeit
```

Inspect only these diffs:
```bash
git diff upstream/main..pr126/main -- dissect/header.go dissect/version.go dissect/feedback.go dissect/defuse.go dissect/scoreboard.go dissect/player.go dissect/stats.go

git diff upstream/main..pr124/improve-defuser-detection -- dissect/defuse.go
```

## What to change

### 1) Add Solid Snake
Main file: `dissect/header.go`

Tasks:
- add `SolidSnake` operator constant in the correct place
- update any source data used to generate operator string / role files
- regenerate generated files instead of hand-editing if possible

Likely generated files:
- `dissect/operator_string.go`
- `dissect/operator_roles.go`

Also check `dissect/version.go` for Y11S1 replay support if needed.

### 2) Add FortressY10
From PR #126, add:
- `FortressY10 Map = 398899676157`

Then regenerate:
- `dissect/map_string.go`

### 3) Fix DBNO finish kill credit
Main file: `dissect/feedback.go`

Desired behavior:
- if player A downs target T
- and player B confirms T while T is DBNO
- official kill goes to A, not B

Use PR #126 as the main idea source.

Prefer:
- track original downer
- when a later finish event occurs on the same target, assign kill to downer
- keep totals consistent with scoreboard semantics
- do not break ordinary non-DBNO kills

### 4) Fix defuser detection
Main file: `dissect/defuse.go`

Use both PRs:
- start with PR #124 for cleaner timer-direction logic
- compare PR #126 for newer replay compatibility

Target behavior:
- identify plant start vs plant complete
- identify disable start vs disable complete
- aborted plant should not count as a completed plant
- timer expiry should resolve to the correct side
- post-plant defender disable should resolve correctly even if attacker dies first
- logic should work from either player's replay perspective

### 5) Ignore a final prep-phase forfeit when parsing a match folder
This is a future feature to implement now if feasible.

Scope:
- applies when input is a match folder / full match, not just a single `.rec`
- if the final round ends because all players on at least one team leave before prep phase ends, that round should be ignored entirely
- ignored means: do not include it in parsed rounds, stats, kills, plants, scoreline progression, or final round count
- this must work whether the replay owner left during that round or stayed until the result screen

This is specifically motivated by `match2` vs `match3` below.

## Local replay fixtures
Root folder:
- `C:\Projects\r6-dissect\test-files`

This folder contains multiple subfolders such as `match1`, `match2`, `match3`.

### match1
Minimal local custom replay.

Ground truth:
- Round 1:
  - only one human player
  - attacker is Solid Snake
  - attacker plants
  - defender does not disable
  - timer expires
  - attackers win
- Round 2:
  - only one human player
  - attacker is Solid Snake
  - Solid Snake dies to his own grenade

Use `match1` mainly to:
- identify Solid Snake's operator ID
- verify operator mapping is correct on a minimal replay
- verify planted-defuser win parsing on a minimal replay

### match2
Online custom match, 1 player on each side.
This replay is from the player who left during the final round.

Ground truth:
- Round 1:
  - Solid Snake starts planting
  - stops before plant completes
  - then kills defender
  - attackers win by elimination
- Round 2:
  - Solid Snake plants
  - defender does not disable
  - timer expires
  - attackers win
- Round 3:
  - attacker plants
  - defender disables
  - defenders win
- Round 4:
  - attacker plants
  - defender kills attacker
  - defender disables
  - defenders win
- Round 5:
  - during prep phase, all players from at least one team leave
  - round ends by forfeit
  - replay owner leaves during prep phase

Use `match2` mainly to validate:
- aborted plant != completed plant
- attacker plant + timer expiry
- defender disable completion
- attacker death before defender disable
- behavior when replay owner leaves during a final-round forfeit

### match3
Same underlying match as `match2`, but replay is from the other player who stayed until the end.

Use `match3` to compare against `match2`:
- defuser detection should still be correct from the other player's replay perspective
- final-round forfeit handling should behave the same when parsing the full match folder

## How to use the replay fixtures
1. Enumerate files under `test-files\match1`, `test-files\match2`, and `test-files\match3`.
2. Run the parser against all replay chunks in each match folder.
3. Use `match1` to discover the unknown operator ID that must map to `SolidSnake`.
4. Use `match2` to confirm plant/disable state transitions round by round.
5. Compare `match2` vs `match3` on the same round outcomes, especially rounds 2 to 5.
6. When parsing a whole match folder, ensure the final prep-phase forfeit round is dropped entirely for both `match2` and `match3`.
7. Add tests from these replays if practical; otherwise add focused unit tests around the new logic.

## How to find Solid Snake's operator ID
Use `match1` first because there is only one player and that player is Solid Snake.

Suggested method:
- trace where operator IDs are read
- add temporary debug output for parsed operator IDs / nearby raw values
- find the unknown ID that consistently belongs to the only attacker in `match1`
- confirm the same ID appears for the attacker in `match2`
- map that ID to `SolidSnake`
- remove debug code before final commit

## What to verify from match2 and match3
Minimum round-level expectations:
- `match2` Round 1: no completed plant
- `match2` Round 2: completed plant, attackers win by timer expiry
- `match2` Round 3: completed plant, defenders win by disable
- `match2` Round 4: completed plant, attacker death does not prevent correct disable detection
- `match2` Round 5: ignored entirely when parsing the whole match folder
- `match3` should produce the same real rounds and same outcomes for rounds 1 to 4
- `match3` final prep-phase forfeit round should also be ignored entirely when parsing the whole match folder

## Regeneration / test commands
```bash
go install golang.org/x/tools/cmd/stringer@latest
export PATH="$(go env GOPATH)/bin:$PATH"
go generate ./...
go test ./...
```

## Minimum acceptable result
- Solid Snake parses by name
- FortressY10 parses by name
- aborted plant is not treated as a completed plant
- attacker plant + timer expiry is parsed correctly
- defender disable is parsed correctly
- logic works from both replay-owner perspectives in `match2` and `match3`
- DBNO confirm kill is credited to the original downer
- when parsing a match folder, the final prep-phase forfeit round is ignored entirely
- tests pass

## Commit scope
Keep the patch small. One PR is fine if it contains:
- operator/map/version updates
- defuser fix
- DBNO kill-credit fix
- final-round forfeit ignore logic for folder parsing
- minimal tests
