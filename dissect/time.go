package dissect

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

func latestMatchUpdate(feedback []MatchUpdate, updateType MatchUpdateType) (MatchUpdate, bool) {
	for i := len(feedback) - 1; i >= 0; i-- {
		if feedback[i].Type == updateType {
			return feedback[i], true
		}
	}
	return MatchUpdate{}, false
}

func readTime(r *Reader) error {
	time, err := r.Uint32()
	if err != nil {
		return err
	}
	r.time = float64(time)
	r.timeRaw = fmt.Sprintf("%d:%02d", time/60, time%60)
	return nil
}

func readY7Time(r *Reader) error {
	time, err := r.String()
	parts := strings.Split(time, ":")
	if len(parts) == 1 {
		seconds, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return err
		}
		r.time = seconds
		r.timeRaw = parts[0]
		return nil
	}
	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	r.time = float64((minutes * 60) + seconds)
	r.timeRaw = time
	return nil
}

func (r *Reader) reconcileKillsWithScoreboard() {
	parsedKills := make(map[string]int, len(r.Header.Players))
	scoreboardKills := make(map[string]int, len(r.Header.Players))
	playerByUsername := make(map[string]Player, len(r.Header.Players))
	killIndexesByUsername := make(map[string][]int, len(r.Header.Players))

	for i, p := range r.Header.Players {
		playerByUsername[p.Username] = p
		if i < len(r.Scoreboard.Players) {
			scoreboardKills[p.Username] = int(r.Scoreboard.Players[i].Kills)
		}
	}
	for i, update := range r.MatchFeedback {
		if update.Type != Kill {
			continue
		}
		parsedKills[update.Username]++
		killIndexesByUsername[update.Username] = append(killIndexesByUsername[update.Username], i)
	}

	missingKills := make(map[string]int)
	surplusKills := make(map[string]int)
	maxAbsDiff := 0
	for _, p := range r.Header.Players {
		username := p.Username
		diff := scoreboardKills[username] - parsedKills[username]
		absDiff := diff
		if absDiff < 0 {
			absDiff = -absDiff
		}
		if absDiff > maxAbsDiff {
			maxAbsDiff = absDiff
		}
		if diff > 0 {
			missingKills[username] = diff
		} else if diff < 0 {
			surplusKills[username] = -diff
		}
		if scoreboardDebugEnabled && r.Header.CodeVersion >= Y11S1 && (scoreboardKills[username] != 0 || parsedKills[username] != 0) {
			log.Debug().
				Str("username", username).
				Int("parsed_kills", parsedKills[username]).
				Int("scoreboard_kills", scoreboardKills[username]).
				Int("kill_diff", diff).
				Msg("scoreboard_reconcile_kill_counts")
		}
	}
	if r.Header.CodeVersion >= Y11S1 && maxAbsDiff > 1 {
		if scoreboardDebugEnabled {
			log.Debug().
				Int("max_abs_diff", maxAbsDiff).
				Msg("scoreboard_reconcile_skipped_untrusted_y11_kills")
		}
		return
	}
	if len(missingKills) == 0 || len(surplusKills) == 0 {
		return
	}

	for username, surplus := range surplusKills {
		indexes := killIndexesByUsername[username]
		for i := len(indexes) - 1; i >= 0 && surplus > 0; i-- {
			killIndex := indexes[i]
			update := &r.MatchFeedback[killIndex]
			killer := playerByUsername[update.Username]
			target := playerByUsername[update.Target]
			if killer.Username == "" || target.Username == "" {
				continue
			}

			candidates := make([]string, 0)
			for candidate, missing := range missingKills {
				if missing == 0 {
					continue
				}
				player := playerByUsername[candidate]
				if player.Username == "" || player.TeamIndex != killer.TeamIndex || candidate == update.Username {
					continue
				}
				candidates = append(candidates, candidate)
			}
			if len(candidates) == 0 {
				continue
			}
			slices.SortFunc(candidates, func(a, b string) int {
				if missingKills[a] != missingKills[b] {
					return missingKills[b] - missingKills[a]
				}
				return strings.Compare(a, b)
			})

			replacement := candidates[0]
			replacementPlayer := playerByUsername[replacement]
			if replacementPlayer.TeamIndex == target.TeamIndex {
				continue
			}

			log.Debug().
				Str("from", update.Username).
				Str("to", replacement).
				Str("target", update.Target).
				Msg("reconciled_kill_with_scoreboard")
			update.Username = replacement
			missingKills[replacement]--
			surplus--
		}
	}
}

func (r *Reader) roundEnd() {
	log.Debug().Msg("round_end")

	r.reconcileKillsWithScoreboard()
	r.reconcileDefuserActors()

	planter := -1
	scoreWinner := -1
	deaths := make(map[int]int)
	sizes := make(map[int]int)
	roles := make(map[int]TeamRole)

	for _, p := range r.Header.Players {
		sizes[p.TeamIndex] += 1
		roles[p.TeamIndex] = r.Header.Teams[p.TeamIndex].Role
	}

	if r.Header.CodeVersion >= Y9S4 {
		team0Won := r.Header.Teams[0].StartingScore < r.Header.Teams[0].Score
		r.Header.Teams[0].Won = team0Won
		r.Header.Teams[1].Won = !team0Won
		if team0Won {
			scoreWinner = 0
		} else {
			scoreWinner = 1
		}
	}

	if _, hasDisableComplete := latestMatchUpdate(r.MatchFeedback, DefuserDisableComplete); !hasDisableComplete && scoreWinner >= 0 {
		if plant, hasPlant := latestMatchUpdate(r.MatchFeedback, DefuserPlantComplete); hasPlant && plant.Username != "" && r.Header.Teams[scoreWinner].Role == Defense {
			if replayUsername := r.usernameForPlayerIndex(r.lastReplayDisablingPlayerIndex); replayUsername != "" {
				r.MatchFeedback = append(r.MatchFeedback, MatchUpdate{
					Type:          DefuserDisableComplete,
					Username:      replayUsername,
					Time:          r.timeRaw,
					TimeInSeconds: r.time,
				})
			} else if disableStart, hasDisableStart := latestMatchUpdate(r.MatchFeedback, DefuserDisableStart); hasDisableStart && disableStart.Username != "" {
				r.MatchFeedback = append(r.MatchFeedback, MatchUpdate{
					Type:          DefuserDisableComplete,
					Username:      disableStart.Username,
					Time:          disableStart.Time,
					TimeInSeconds: disableStart.TimeInSeconds,
				})
			}
		}
	}
	if _, hasDisableComplete := latestMatchUpdate(r.MatchFeedback, DefuserDisableComplete); !hasDisableComplete {
		if plant, hasPlant := latestMatchUpdate(r.MatchFeedback, DefuserPlantComplete); hasPlant && plant.Username != "" {
			if disableStart, hasDisableStart := latestMatchUpdate(r.MatchFeedback, DefuserDisableStart); hasDisableStart && disableStart.Username != "" {
				r.MatchFeedback = append(r.MatchFeedback, MatchUpdate{
					Type:          DefuserDisableComplete,
					Username:      disableStart.Username,
					Time:          disableStart.Time,
					TimeInSeconds: disableStart.TimeInSeconds,
				})
			}
		}
	}

	for _, u := range r.MatchFeedback {
		switch u.Type {
		case Kill:
			targetIndex := r.PlayerIndexByUsername(u.Target)
			if targetIndex < 0 {
				break
			}
			i := r.Header.Players[targetIndex].TeamIndex
			deaths[i] = deaths[i] + 1
			break
		case Death:
			playerIndex := r.PlayerIndexByUsername(u.Username)
			if playerIndex < 0 {
				break
			}
			i := r.Header.Players[playerIndex].TeamIndex
			deaths[i] = deaths[i] + 1
			break
		case DefuserPlantComplete:
			if playerIndex := r.PlayerIndexByUsername(u.Username); playerIndex >= 0 {
				planter = playerIndex
			}
			break
		case DefuserDisableComplete:
			playerIndex := r.PlayerIndexByUsername(u.Username)
			if playerIndex < 0 {
				break
			}
			i := r.Header.Players[playerIndex].TeamIndex
			r.Header.Teams[i].Won = true
			r.Header.Teams[i^1].Won = false
			r.Header.Teams[i].WinCondition = DisabledDefuser
			return
		}
	}

	if planter > -1 {
		planterTeam := r.Header.Players[planter].TeamIndex
		if scoreWinner == -1 || scoreWinner == planterTeam {
			r.Header.Teams[planterTeam].Won = true
			r.Header.Teams[planterTeam^1].Won = false
			r.Header.Teams[planterTeam].WinCondition = DefusedBomb
			return
		}
		r.Header.Teams[scoreWinner].Won = true
		r.Header.Teams[scoreWinner^1].Won = false
		r.Header.Teams[scoreWinner].WinCondition = DisabledDefuser
		return
	}

	// skip for now until we have a more reliable way of determining the win condition
	// Y9S4 at least tells us who won now in the header with StartingScore
	if r.Header.CodeVersion >= Y9S4 {
		return
	}

	if deaths[0] == sizes[0] {
		if planter > -1 && roles[0] == Attack { // ignore attackers killed post-plant
			return
		}
		r.Header.Teams[1].Won = true
		r.Header.Teams[1].WinCondition = KilledOpponents
		return
	}
	if deaths[1] == sizes[1] {
		if planter > -1 && roles[1] == Attack { // ignore attackers killed post-plant
			return
		}
		r.Header.Teams[0].Won = true
		r.Header.Teams[0].WinCondition = KilledOpponents
		return
	}

	i := 0
	if roles[1] == Defense {
		i = 1
	}

	r.Header.Teams[i].Won = true
	r.Header.Teams[i].WinCondition = Time
}
