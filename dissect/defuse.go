package dissect

import (
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

func (r *Reader) getTeamByRole(role TeamRole) int {
	for i, team := range r.Header.Teams {
		if team.Role == role {
			return i
		}
	}
	return -1
}

func (r *Reader) getAlivePlayersByTeam(teamIndex int) []string {
	alive := make([]string, 0, 5)
	for _, p := range r.Header.Players {
		if p.TeamIndex != teamIndex {
			continue
		}
		died := false
		for _, fb := range r.MatchFeedback {
			if fb.Type == Kill && fb.Target == p.Username {
				died = true
				break
			}
			if fb.Type == Death && fb.Username == p.Username {
				died = true
				break
			}
		}
		if !died && p.Username != "" {
			alive = append(alive, p.Username)
		}
	}
	return alive
}

func readDefuserTimer(r *Reader) error {
	timer, err := r.String()
	if err != nil {
		return err
	}
	prevTimer := r.lastDefuserTimer
	timerValue := -1.0
	if len(timer) > 0 {
		if v, parseErr := strconv.ParseFloat(timer, 64); parseErr == nil {
			timerValue = v
		}
	}
	playerIndex := -1
	if r.Header.CodeVersion >= Y10S4 {
		targetRole := Attack
		if r.planted {
			targetRole = Defense
		}
		teamIndex := r.getTeamByRole(targetRole)
		if teamIndex >= 0 {
			alive := r.getAlivePlayersByTeam(teamIndex)
			if len(alive) == 1 {
				playerIndex = r.PlayerIndexByUsername(alive[0])
			}
		}
	} else {
		if err = r.Skip(34); err != nil {
			return err
		}
		id, err := r.Bytes(4)
		if err != nil {
			return err
		}
		playerIndex = r.PlayerIndexByID(id)
	}
	updateType := DefuserPlantStart
	recordStartEvent := true
	if r.planted {
		if timerValue >= 0 && prevTimer >= 0 && timerValue > prevTimer {
			updateType = DefuserDisableStart
			r.defuserDisabling = true
		} else {
			recordStartEvent = false
		}
	} else {
		r.defuserDisabling = false
	}
	if recordStartEvent && playerIndex > -1 && r.lastDefuserPlayerIndex != playerIndex {
		u := MatchUpdate{
			Type:          updateType,
			Username:      r.Header.Players[playerIndex].Username,
			Time:          r.timeRaw,
			TimeInSeconds: r.time,
		}
		r.MatchFeedback = append(r.MatchFeedback, u)
		log.Debug().Interface("match_update", u).Send()
		r.lastDefuserPlayerIndex = playerIndex
	}
	if !strings.HasPrefix(timer, "0.00") {
		r.lastDefuserTimer = timerValue
		return nil
	}
	updateType = DefuserDisableComplete
	if !r.planted {
		updateType = DefuserPlantComplete
		r.planted = true
		r.defuserDisabling = false
	} else if r.defuserDisabling {
		r.defuserDisabling = false
		r.planted = false
	} else {
		r.lastDefuserTimer = timerValue
		return nil
	}
	username := ""
	if r.lastDefuserPlayerIndex >= 0 && r.lastDefuserPlayerIndex < len(r.Header.Players) {
		username = r.Header.Players[r.lastDefuserPlayerIndex].Username
	}
	u := MatchUpdate{
		Type:          updateType,
		Username:      username,
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
	r.lastDefuserTimer = timerValue
	return nil
}
