# Codex notes: defuser / planter bug analysis

Use the repo’s normal entrypoints for single-round `.rec` parsing and folder parsing. The upstream tool supports both a match folder and a single `.rec` file as inputs. :contentReference[oaicite:0]{index=0}

## Relevant test folders

- `test-files\match1`
- `test-files\match2`
- `test-files\match3`
- `test-files\match4`
- `test-files\match5`

## Ground truth

### match1

Use for minimal sanity checks and Solid Snake operator work.

### match2

Use for normal 1v1 online custom match behavior and defuser-state transitions.

- R1: attacker starts planting, cancels, kills defender, attackers win
- R2: attacker plants, no defuse, timer expires, attackers win
- R3: attacker plants, defender defuses, defenders win
- R4: attacker plants, defender kills attacker, defender defuses, defenders win
- R5: prep-phase forfeit; replay owner leaves during prep

### match3

Same match as `match2`, but from the other player’s replay.
Use to compare opposite-perspective behavior.

### match4

Treat this like `match5`: another chaotic match to use as a falsification case, not a place to loosen broad heuristics.
Use it to verify that plant/disable attribution still holds in crowded rounds.

Ground truth for known rounds:

- R2:
  - `OreoSenpai` plants the defuser
  - `Rival-_` kills `OreoSenpai`
  - `Rival-_` Starts to disable the defuser, but stops midway
  - `Rival-_` kills `CherryKnives`
  - Round ends on timeout, attackers win
- R5:
  - `IceDogs2003` plants the defuser
  - `CherryKnives` kills `IceDogs2003`
  - `CherryKnives` kills `LumpFlame706`
  - `OreoSenpai` disables the defuser
  - `CherryKnives` (defender) and `Papi_RumRum` (attacker) are also alive when disable completes

### match5

Ignore all rounds except round 4.
Treat that round as `roundNumber = 3`.

Ground truth for that round:

- `OreoSenpai` plants the defuser
- `ironsmithers` kills `OreoSenpai`
- `ironsmithers` kills `Otter.ETS`
- `ironsmithers` disables the defuser
- defenders win

### match6

Treat this like `match5`: another chaotic match to use as a falsification case, not a place to loosen broad heuristics.
Use it to verify that plant/disable attribution still holds in crowded rounds.

Ground truth for known rounds:
Round 3:

- `Daddy.Mozzie` plants the defuser
- `Daddy.Mozzie` kills `PromizeYT`
- `WHITE.o` kills `Daddy.Mozzie`
- `ChudChar` kills `WHITE.o`
- Attackers win on kills (after plant complete, not a win on timer though)

### match7

Treat this like `match5`: another chaotic match to use as a falsification case, not a place to loosen broad heuristics.
Use it to verify that plant/disable attribution still holds in crowded rounds.

- `PercSpinner` plants the defuser
- `LundyTrades` kills `PercSpinner`
- `OO1..` kills `skipalangdang`
- `LundyTrades` disables the defuser
- defenders win

### match8

This is a clean match with only 1 player on each team for testing dbno bleeding out.

Ground truth for known rounds:
Round 3:

- `Mori.CU` downs `MoriRecruit` via body shots
- `MoriRecruit` bleeds out
- Attackers win on kills
- Note MoriRecruit has rook armor in this round, which allows players to always enter dbno state when hp goes below 0 with a non-headshot reason. This may impact the file.

Round 4:

- `Mori.CU` downs `MoriRecruit`
- `MoriRecruit` bleeds out
- Attackers win on kills
- Note MoriRecruit does not have rook armor, so would only go into dnbo state if hp goes between 0 and -10, which is what happens in this round due to Mori.CU.

### match9

This is a chaotic match.

Ground truth for known rounds:
Round 1:

- Attacker `Mori.CU` with nickname `quirkyUtensil` downs `Tyanomi` via body shots through a wall (through the wall may or may not be relevant) in a 4v4 situation(4 defenders including the dbno state `Tyanomi` alive)
- 2 attackers get killed, one of whom leaves the lobby.
- `Tyanomi` gets revived by `massaman`
- Attacker `Mori.CU` with nickname `quirkyUtensil` downs `MKAQU416`
- Attacker `Mori.CU` with nickname `quirkyUtensil` downs `massaman`
- Attacker `kuroyuri00` kills `Tyanomi` (credit of killing Tyanomi should belong to kuroyuri00 since they got revived.)
- Attacker `Mori.CU` with nickname `quirkyUtensil` kills `Arison_KURONURI`
- As a result, the round is won by attackers on kills and `massaman` and `MKAQU416` who were downed earlier by `quirkyUtensil` die and the kills get credited to `quirkyUtensil`

Round 2:

- Attacker `Mori.CU` with nickname `quirkyUtensil` downs `Arison_KURONURI` in a 3v3 situation. (3 defenders including the dbno state`Arison_KURONURI` alive)
- Defender `massaman` downs `bird135` and quickly confirms the kill themselves (downer and confirmer are same person)
- Attacker `hagegore` confirms the kill on `Arison_KURONURI` and `quirkyUtensil` gets credit for the kill
- Attacker `hagegore` downs `massaman` and quickly confirms the kill themselves (downer and confirmer are same person)
- Rest of round is uninterestings

Round 4:

- Defenders lead 5v2
- Defender`Mori.CU` with nickname `quirkyUtensil` downs `massaman`
- Attacker `Arison_KURONURI` downs `quirkyUtensil`
- Defender `bird135` confirms kill on `massaman` confirming the kill for `quirkyUtensil`
- Attacker `Arison_KURONURI` downs `bird135` and quickly confirms the kill themselves (downer and confirmer are same person)
- Attacker `Arison_KURONURI` gets normal kill
- Defender kills `Arison_KURONURI` and defenders win on kills while `quirkyUtensil` is still down (and does not die due to round win so no kill credit should be assigned to `Arison_KURONURI` who downed `quirkyUtensil`)

## Main suspicion

Current planter/disabler attribution is likely too dependent on a nearby kill event.
That is unsafe because:

- the closest kill may involve the defender
- the planter is not always the first player to die after plant
- another kill can happen before the planter appears in later events

## Code areas to inspect

Primary:

- `dissect/defuse.go`

Secondary:

- `dissect/feedback.go`
- tests under `dissect/test`

## What to verify first

1. Whether plant attribution and disable attribution share the same temporary actor state
2. Whether actor choice is based on nearest kill event
3. Whether planter/disabler side validity is checked
4. Whether actor is chosen at action start or only inferred later

## Best first fix to try

Do this before trying anything fancy:

1. Separate stored actor state:
   - `lastPlantingPlayerIndex`
   - `lastDisablingPlayerIndex`

2. Lock actor at action start, not completion:
   - when plant starts, choose and store planter
   - when disable starts, choose and store disabler
   - completion reuses stored actor

3. Enforce side rules:
   - planter must be attacker
   - disabler must be defender

4. Downgrade kill-event proximity to fallback only, not primary evidence

## Strong rules

- Never assign a defender as planter
- Never assign an attacker as disabler
- Plant actor must not be overwritten by later kill events
- `match2` and `match3` must produce the same logical attribution

## Suggested debugging

Add temporary logs around plant/disable state transitions:

- round number
- replay time
- inferred event:
  - `plant_start`
  - `plant_complete`
  - `disable_start`
  - `disable_complete`
- chosen player index
- chosen username
- chosen side
- reason for choice
- nearby kill events considered

Goal:

- explain exactly why `match5` picks `ironsmithers`

## Minimum regression checks

Must pass:

- `match5` logical round 3:
  - planter = `OreoSenpai`
  - disabler = `ironsmithers`

- `match2` R2:
  - planter set correctly
  - no disabler
  - attackers win by timer expiry

- `match2` R3:
  - planter = attacker
  - disabler = defender

- `match2` R4:
  - planter remains attacker even though attacker dies before disable
  - disabler = defender

- `match2` and `match3` must agree from both replay perspectives

Must not regress:

- `match2` R1 aborted plant must not become a completed plant
- `match1` simple timer behavior must still parse correctly

## Future compatibility

Keep this compatible with the folder-level rule for final-round prep-phase forfeits:
when parsing a whole match folder, if the final round ends by prep-phase forfeit, that final round should be ignored entirely regardless of replay-owner perspective.

## Practical approach

Recommended order:

1. add debug logs
2. reproduce `match5`
3. add separate plant/disable actor state
4. add side validation
5. rerun `match2`, `match3`, `match5`
6. only then consider stronger signals such as direct interaction actor or defuser ownership if replay data exposes them
