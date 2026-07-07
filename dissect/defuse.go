package dissect

import (
	"bytes"
	"encoding/binary"
	"slices"
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
	return r.getAlivePlayersByTeamAtTime(teamIndex, -1)
}

func (r *Reader) getAlivePlayersByTeamAtTime(teamIndex int, timeInSeconds float64) []string {
	alive := make([]string, 0, 5)
	for _, p := range r.Header.Players {
		if p.TeamIndex != teamIndex {
			continue
		}
		died := false
		for _, fb := range r.MatchFeedback {
			if timeInSeconds >= 0 && fb.TimeInSeconds <= timeInSeconds {
				continue
			}
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

func (r *Reader) playerIndexForDefuserRole(role TeamRole) int {
	return r.playerIndexForDefuserRoleAtTime(role, -1)
}

func (r *Reader) playerIndexForDefuserRoleAtTime(role TeamRole, timeInSeconds float64) int {
	teamIndex := r.getTeamByRole(role)
	if teamIndex < 0 {
		return -1
	}
	alive := r.getAlivePlayersByTeamAtTime(teamIndex, timeInSeconds)
	if len(alive) != 1 {
		return -1
	}
	return r.PlayerIndexByUsername(alive[0])
}

func (r *Reader) playerIndexForRoleFromPacket(role TeamRole, timeInSeconds float64) (int, error) {
	if r.Header.CodeVersion >= Y10S4 {
		return r.playerIndexForDefuserRoleAtTime(role, timeInSeconds), nil
	}
	if err := r.Skip(34); err != nil {
		return -1, err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return -1, err
	}
	playerIndex := r.PlayerIndexByID(id)
	if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
		return -1, nil
	}
	player := r.Header.Players[playerIndex]
	teamRole := r.Header.Teams[player.TeamIndex].Role
	if teamRole != role {
		return -1, nil
	}
	return playerIndex, nil
}

func entityRefValueAt(b []byte, i int) (uint64, bool) {
	if i < 0 || i+9 > len(b) {
		return 0, false
	}
	if b[i] != 0x23 && b[i] != 0x1B {
		return 0, false
	}
	if b[i+4] != 0xF0 || b[i+5] != 0x00 || b[i+6] != 0x00 || b[i+7] != 0x00 || b[i+8] != 0x00 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(b[i+1 : i+9]), true
}

func (r *Reader) registerPlayerEntityRefs(playerIndex int, usernameStart int, usernameEnd int) {
	if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
		return
	}
	start := usernameStart - 256
	if start < 0 {
		start = 0
	}
	end := usernameEnd + 1024
	if end > len(r.b) {
		end = len(r.b)
	}
	for i := start; i+9 <= end; i++ {
		ref, ok := entityRefValueAt(r.b, i)
		if !ok {
			continue
		}
		registerEntityRefOwner(ref, playerIndex, r.playerEntityRefOwners, r.playerEntityRefConflicts)
	}
}

func registerEntityRefOwner(ref uint64, playerIndex int, owners map[uint64]int, conflicts map[uint64]struct{}) {
	if _, conflict := conflicts[ref]; conflict {
		return
	}
	if owner, found := owners[ref]; found && owner != playerIndex {
		delete(owners, ref)
		conflicts[ref] = struct{}{}
		return
	}
	owners[ref] = playerIndex
}

func (r *Reader) registerPlayerTimerAliasRefs(packetStart int, packetEnd int) {
	start := packetStart - 64
	if start < 0 {
		start = 0
	}
	end := packetEnd + 96
	if end > len(r.b) {
		end = len(r.b)
	}
	for i := start; i+34 <= end; i++ {
		ownerRef, ok := entityRefValueAt(r.b, i)
		if !ok {
			continue
		}
		playerIndex, found := r.playerEntityRefOwners[ownerRef]
		if !found {
			continue
		}
		if !bytes.Equal(r.b[i+9:i+13], []byte{0x27, 0xC0, 0x8D, 0xCA}) {
			continue
		}
		if !bytes.Equal(r.b[i+21:i+25], []byte{0xB2, 0x21, 0x6B, 0xF3}) {
			continue
		}
		aliasRef, ok := entityRefValueAt(r.b, i+25)
		if !ok || aliasRef == ownerRef {
			continue
		}
		registerEntityRefOwner(aliasRef, playerIndex, r.playerTimerAliasRefOwners, r.playerTimerAliasRefConflicts)
	}
}

func (r *Reader) playerIndexAliveAtTime(playerIndex int, timeInSeconds float64) bool {
	if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
		return false
	}
	username := r.Header.Players[playerIndex].Username
	for _, fb := range r.MatchFeedback {
		if fb.TimeInSeconds <= timeInSeconds {
			continue
		}
		if (fb.Type == Kill && fb.Target == username) || (fb.Type == Death && fb.Username == username) {
			return false
		}
	}
	return true
}

func (r *Reader) playerIndexFromObjectiveUsernameWindow(role TeamRole, timeInSeconds float64, start int, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(r.b) {
		end = len(r.b)
	}
	if start >= end {
		return -1
	}
	bestPlayerIndex := -1
	bestOffset := end + 1
	for i := start; i+28 <= end; i++ {
		if _, ok := entityRefValueAt(r.b, i); !ok {
			continue
		}
		payload := r.b[i+9:]
		if len(payload) < 19 || !bytes.HasPrefix(payload, []byte{
			0xDE, 0xAE, 0xCA, 0x09,
			0x01, 0x00,
			0x22, 0x49, 0xD8, 0x60, 0x4F,
			0x01, 0x00,
			0x22, 0x5B, 0xE8, 0x47, 0x28,
		}) {
			continue
		}
		size := int(payload[18])
		nameStart := i + 9 + 19
		nameEnd := nameStart + size
		if size <= 0 || nameEnd > end {
			continue
		}
		playerIndex := r.PlayerIndexByUsername(string(r.b[nameStart:nameEnd]))
		if !r.playerIndexHasRole(playerIndex, role) || !r.playerIndexAliveAtTime(playerIndex, timeInSeconds) {
			continue
		}
		if i < bestOffset {
			bestOffset = i
			bestPlayerIndex = playerIndex
		}
	}
	return bestPlayerIndex
}

func (r *Reader) playerIndexFromEntityRefWindow(role TeamRole, start int, end int) int {
	return r.playerIndexFromOwnedEntityRefWindow(role, start, end, r.playerEntityRefOwners)
}

func (r *Reader) playerIndexFromOwnedEntityRefWindow(role TeamRole, start int, end int, owners map[uint64]int) int {
	if start < 0 {
		start = 0
	}
	if end > len(r.b) {
		end = len(r.b)
	}
	if start >= end {
		return -1
	}
	type candidate struct {
		count       int
		firstOffset int
	}
	candidates := map[int]candidate{}
	for i := start; i+9 <= end; i++ {
		ref, ok := entityRefValueAt(r.b, i)
		if !ok {
			continue
		}
		playerIndex, found := owners[ref]
		if !found || !r.playerIndexHasRole(playerIndex, role) {
			continue
		}
		entry := candidates[playerIndex]
		entry.count++
		if entry.firstOffset == 0 {
			entry.firstOffset = i
		}
		if i < entry.firstOffset {
			entry.firstOffset = i
		}
		candidates[playerIndex] = entry
	}
	bestPlayerIndex := -1
	bestCount := 0
	bestOffset := end + 1
	for playerIndex, candidate := range candidates {
		if candidate.count > bestCount || (candidate.count == bestCount && candidate.firstOffset < bestOffset) {
			bestPlayerIndex = playerIndex
			bestCount = candidate.count
			bestOffset = candidate.firstOffset
		}
	}
	return bestPlayerIndex
}

func (r *Reader) resolvePlayerIndexFromTimerWindow(role TeamRole, timeInSeconds float64, packetStart int, packetEnd int) int {
	if r.Header.CodeVersion >= Y11S2 {
		return r.playerIndexFromY11S2DefuserTimerActorRef(packetStart)
	}
	if role == Defense && len(r.Header.Players) <= 2 {
		if playerIndex := r.playerIndexFromOwnedEntityRefWindow(role, packetStart-32, packetEnd, r.playerTimerAliasRefOwners); playerIndex >= 0 {
			return playerIndex
		}
	}
	if role == Defense {
		if playerIndex := r.playerIndexFromObjectiveUsernameWindow(role, timeInSeconds, packetEnd, packetEnd+256); playerIndex >= 0 {
			return playerIndex
		}
		if playerIndex := r.playerIndexFromEntityRefWindow(role, packetEnd, packetEnd+64); playerIndex >= 0 {
			return playerIndex
		}
	} else {
		if playerIndex := r.playerIndexFromObjectiveUsernameWindow(role, timeInSeconds, packetEnd, packetEnd+256); playerIndex >= 0 {
			return playerIndex
		}
		if playerIndex := r.playerIndexFromOwnedEntityRefWindow(role, packetStart-32, packetEnd, r.playerTimerAliasRefOwners); playerIndex >= 0 {
			return playerIndex
		}
	}
	if len(r.Header.Players) > 2 {
		return -1
	}
	return r.playerIndexFromEntityRefWindow(role, packetStart-32, packetEnd+256)
}

func (r *Reader) playerIndexFromY11S2DefuserTimerActorRef(packetStart int) int {
	for _, relativeOffset := range []int{36, 11} {
		ref, ok := entityRefValueAt(r.b, packetStart+relativeOffset)
		if !ok {
			continue
		}
		if playerIndex, found := r.y11s2EntityRefOwnerIndex(ref); found {
			return playerIndex
		}
		if playerIndex, found := r.playerEntityRefOwners[ref]; found {
			return playerIndex
		}
	}
	return -1
}

func (r *Reader) y11s2EntityRefOwnerIndex(ref uint64) (int, bool) {
	if r.y11s2EntityRefOwners == nil {
		r.y11s2EntityRefOwners = r.buildY11S2EntityRefOwners()
	}
	playerIndex, found := r.y11s2EntityRefOwners[ref]
	return playerIndex, found
}

func (r *Reader) buildY11S2EntityRefOwners() map[uint64]int {
	type refCount map[int]int
	counts := make(map[uint64]refCount)
	for playerIndex, player := range r.Header.Players {
		name := player.Username
		if name == "" || len(name) > 255 {
			continue
		}
		raw := []byte(name)
		searchStart := 0
		for searchStart+len(raw) <= len(r.b) {
			relative := bytes.Index(r.b[searchStart:], raw)
			if relative < 0 {
				break
			}
			nameStart := searchStart + relative
			nameEnd := nameStart + len(raw)
			searchStart = nameStart + 1
			if nameStart == 0 || int(r.b[nameStart-1]) != len(raw) {
				continue
			}
			start := nameStart - 512
			if start < 0 {
				start = 0
			}
			end := nameEnd + 2048
			if end > len(r.b) {
				end = len(r.b)
			}
			for i := start; i+9 <= end; i++ {
				ref, ok := entityRefValueAt(r.b, i)
				if !ok {
					continue
				}
				if counts[ref] == nil {
					counts[ref] = make(refCount)
				}
				counts[ref][playerIndex]++
			}
		}
	}

	owners := make(map[uint64]int)
	for ref, playerCounts := range counts {
		bestPlayerIndex := -1
		bestCount := 0
		secondBestCount := 0
		for playerIndex, count := range playerCounts {
			if count > bestCount {
				secondBestCount = bestCount
				bestCount = count
				bestPlayerIndex = playerIndex
			} else if count > secondBestCount {
				secondBestCount = count
			}
		}
		if bestPlayerIndex < 0 {
			continue
		}
		if len(playerCounts) == 1 || (bestCount >= 2 && bestCount > secondBestCount*2) {
			owners[ref] = bestPlayerIndex
		}
	}
	return owners
}

func (r *Reader) usernameForPlayerIndex(playerIndex int) string {
	if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
		return ""
	}
	return r.Header.Players[playerIndex].Username
}

func (r *Reader) playerIndexHasRole(playerIndex int, role TeamRole) bool {
	if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
		return false
	}
	teamIndex := r.Header.Players[playerIndex].TeamIndex
	if teamIndex < 0 || teamIndex >= len(r.Header.Teams) {
		return false
	}
	return r.Header.Teams[teamIndex].Role == role
}

func (r *Reader) inferPlantCompletionPlayerIndex(timeInSeconds float64) int {
	attackTeam := r.getTeamByRole(Attack)
	if attackTeam < 0 {
		return -1
	}
	bestPlayerIndex := -1
	bestTime := -1.0
	for _, update := range r.MatchFeedback {
		var username string
		switch update.Type {
		case Kill:
			username = update.Target
		case Death:
			username = update.Username
		default:
			continue
		}
		playerIndex := r.PlayerIndexByUsername(username)
		if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
			continue
		}
		player := r.Header.Players[playerIndex]
		if player.TeamIndex != attackTeam {
			continue
		}
		if update.TimeInSeconds > timeInSeconds {
			continue
		}
		if update.TimeInSeconds > bestTime {
			bestTime = update.TimeInSeconds
			bestPlayerIndex = playerIndex
		}
	}
	return bestPlayerIndex
}

func (r *Reader) inferPlayerIndexFromObjectiveStart(updateType MatchUpdateType, timeInSeconds float64, role TeamRole) int {
	bestPlayerIndex := -1
	bestTime := -1.0
	for _, update := range r.MatchFeedback {
		if update.Type != updateType || update.TimeInSeconds < timeInSeconds {
			continue
		}
		playerIndex := r.PlayerIndexByUsername(update.Username)
		if !r.playerIndexHasRole(playerIndex, role) {
			continue
		}
		if update.TimeInSeconds > bestTime {
			bestTime = update.TimeInSeconds
			bestPlayerIndex = playerIndex
		}
	}
	return bestPlayerIndex
}

func (r *Reader) inferDisableCompletionPlayerIndex(timeInSeconds float64) int {
	defenseTeam := r.getTeamByRole(Defense)
	attackTeam := r.getTeamByRole(Attack)
	if defenseTeam < 0 || attackTeam < 0 {
		return -1
	}
	plantTime := -1.0
	for _, update := range r.MatchFeedback {
		if update.Type != DefuserPlantComplete || update.TimeInSeconds < timeInSeconds {
			continue
		}
		if update.TimeInSeconds > plantTime {
			plantTime = update.TimeInSeconds
		}
	}
	bestPlayerIndex := -1
	bestTime := -1.0
	for _, update := range r.MatchFeedback {
		if update.Type != Kill || update.TimeInSeconds < timeInSeconds {
			continue
		}
		if plantTime >= 0 && update.TimeInSeconds > plantTime {
			continue
		}
		killerIndex := r.PlayerIndexByUsername(update.Username)
		targetIndex := r.PlayerIndexByUsername(update.Target)
		if killerIndex < 0 || targetIndex < 0 {
			continue
		}
		if r.Header.Players[killerIndex].TeamIndex != defenseTeam || r.Header.Players[targetIndex].TeamIndex != attackTeam {
			continue
		}
		if update.TimeInSeconds > bestTime {
			bestTime = update.TimeInSeconds
			bestPlayerIndex = killerIndex
		}
	}
	return bestPlayerIndex
}

func (r *Reader) deterministicTeamFallbackPlayerIndex(role TeamRole, timeInSeconds float64) int {
	teamIndex := r.getTeamByRole(role)
	if teamIndex < 0 {
		return -1
	}
	alive := r.getAlivePlayersByTeamAtTime(teamIndex, timeInSeconds)
	if len(alive) == 0 {
		for _, p := range r.Header.Players {
			if p.TeamIndex == teamIndex && p.Username != "" {
				alive = append(alive, p.Username)
			}
		}
	}
	if len(alive) == 0 {
		return -1
	}

	bestPlayerIndex := -1
	bestTime := -1.0
	for _, update := range r.MatchFeedback {
		if update.TimeInSeconds < timeInSeconds {
			continue
		}
		playerIndex := r.PlayerIndexByUsername(update.Username)
		if playerIndex < 0 || playerIndex >= len(r.Header.Players) {
			continue
		}
		if r.Header.Players[playerIndex].TeamIndex != teamIndex {
			continue
		}
		if !slices.Contains(alive, update.Username) {
			continue
		}
		if bestPlayerIndex == -1 || update.TimeInSeconds < bestTime {
			bestPlayerIndex = playerIndex
			bestTime = update.TimeInSeconds
		}
	}
	if bestPlayerIndex >= 0 {
		return bestPlayerIndex
	}

	slices.Sort(alive)
	return r.PlayerIndexByUsername(alive[0])
}

func (r *Reader) completionPlayerIndexWithFallback(updateType MatchUpdateType, currentPlayerIndex int, timeInSeconds float64) int {
	if r.Header.CodeVersion >= Y11S2 && currentPlayerIndex >= 0 {
		return currentPlayerIndex
	}
	role := Attack
	startType := DefuserPlantStart
	if updateType == DefuserDisableComplete {
		role = Defense
		startType = DefuserDisableStart
	}
	if r.playerIndexHasRole(currentPlayerIndex, role) {
		return currentPlayerIndex
	}

	playerIndex := r.inferPlayerIndexFromObjectiveStart(startType, timeInSeconds, role)
	if playerIndex < 0 {
		if role == Attack {
			playerIndex = r.inferPlantCompletionPlayerIndex(timeInSeconds)
		} else {
			playerIndex = r.inferDisableCompletionPlayerIndex(timeInSeconds)
		}
	}
	if playerIndex < 0 {
		playerIndex = r.deterministicTeamFallbackPlayerIndex(role, timeInSeconds)
	}
	return playerIndex
}

func (r *Reader) reconcileDefuserActors() {
	for i := range r.MatchFeedback {
		update := &r.MatchFeedback[i]
		switch update.Type {
		case DefuserPlantComplete:
			playerIndex := r.completionPlayerIndexWithFallback(update.Type, r.PlayerIndexByUsername(update.Username), update.TimeInSeconds)
			if playerIndex >= 0 {
				update.Username = r.Header.Players[playerIndex].Username
			}
		case DefuserDisableComplete:
			playerIndex := r.completionPlayerIndexWithFallback(update.Type, r.PlayerIndexByUsername(update.Username), update.TimeInSeconds)
			if playerIndex >= 0 {
				update.Username = r.Header.Players[playerIndex].Username
			}
		}
	}
}

func readDefuserTimer(r *Reader) error {
	packetStart := r.offset - 5
	timer, err := r.String()
	if err != nil {
		return err
	}
	r.registerPlayerTimerAliasRefs(packetStart, r.offset)
	prevTimer := r.lastDefuserTimer
	timerValue := -1.0
	if len(timer) > 0 {
		if v, parseErr := strconv.ParseFloat(timer, 64); parseErr == nil {
			timerValue = v
		}
	}
	if timerValue < 0 {
		return nil
	}
	newPlantAttempt := !r.planted && prevTimer >= 0 && timerValue > prevTimer+2
	if newPlantAttempt {
		r.lastPlantingPlayerIndex = -1
		r.lastReplayPlantingPlayerIndex = -1
	}
	actionRole := Attack
	if r.planted {
		actionRole = Defense
	}
	playerIndex := -1
	if r.Header.CodeVersion < Y11S2 {
		packetPlayerIndex, err := r.playerIndexForRoleFromPacket(actionRole, r.time)
		if err != nil {
			return err
		}
		playerIndex = packetPlayerIndex
	}
	if replayPlayerIndex := r.resolvePlayerIndexFromTimerWindow(actionRole, r.time, packetStart, r.offset); replayPlayerIndex >= 0 {
		if actionRole == Attack {
			if r.Header.CodeVersion >= Y11S2 &&
				r.lastPlantingPlayerIndex >= 0 &&
				r.lastPlantingPlayerIndex != replayPlayerIndex &&
				!r.playerIndexAliveAtTime(r.lastPlantingPlayerIndex, r.time) {
				r.lastPlantingPlayerIndex = -1
				r.lastReplayPlantingPlayerIndex = -1
			}
			if r.lastPlantingPlayerIndex < 0 || r.lastPlantingPlayerIndex == replayPlayerIndex {
				r.lastReplayPlantingPlayerIndex = replayPlayerIndex
			}
		} else {
			if r.Header.CodeVersion >= Y11S2 &&
				r.lastDisablingPlayerIndex >= 0 &&
				r.lastDisablingPlayerIndex != replayPlayerIndex &&
				!r.playerIndexAliveAtTime(r.lastDisablingPlayerIndex, r.time) {
				r.lastDisablingPlayerIndex = -1
				r.lastReplayDisablingPlayerIndex = -1
			}
			if r.lastDisablingPlayerIndex < 0 || r.lastDisablingPlayerIndex == replayPlayerIndex {
				r.lastReplayDisablingPlayerIndex = replayPlayerIndex
			}
		}
		playerIndex = replayPlayerIndex
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

	if recordStartEvent && playerIndex > -1 {
		lastPlayerIndex := r.lastPlantingPlayerIndex
		if updateType == DefuserDisableStart {
			lastPlayerIndex = r.lastDisablingPlayerIndex
		}
		if lastPlayerIndex == -1 {
			u := MatchUpdate{
				Type:          updateType,
				Username:      r.Header.Players[playerIndex].Username,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
			if updateType == DefuserDisableStart {
				r.lastDisablingPlayerIndex = playerIndex
			} else {
				r.lastPlantingPlayerIndex = playerIndex
				r.lastDisablingPlayerIndex = -1
				r.lastReplayDisablingPlayerIndex = -1
			}
		}
	}

	if !strings.HasPrefix(timer, "0.00") {
		r.lastDefuserTimer = timerValue
		return nil
	}

	updateType = DefuserDisableComplete
	completionPlayerIndex := r.lastReplayDisablingPlayerIndex
	if completionPlayerIndex < 0 {
		completionPlayerIndex = r.lastDisablingPlayerIndex
	}
	if !r.planted {
		updateType = DefuserPlantComplete
		r.planted = true
		r.defuserDisabling = false
		completionPlayerIndex = r.lastReplayPlantingPlayerIndex
		if completionPlayerIndex < 0 {
			completionPlayerIndex = r.lastPlantingPlayerIndex
		}
		if completionPlayerIndex < 0 {
			completionPlayerIndex = playerIndex
		}
		r.lastReplayDisablingPlayerIndex = -1
	} else if r.defuserDisabling || completionPlayerIndex >= 0 {
		r.defuserDisabling = false
		r.planted = false
		if completionPlayerIndex < 0 {
			completionPlayerIndex = playerIndex
		}
		r.lastDisablingPlayerIndex = -1
		r.lastReplayPlantingPlayerIndex = -1
		r.lastReplayDisablingPlayerIndex = -1
	} else {
		r.lastDefuserTimer = timerValue
		return nil
	}

	completionPlayerIndex = r.completionPlayerIndexWithFallback(updateType, completionPlayerIndex, r.time)
	username := r.usernameForPlayerIndex(completionPlayerIndex)
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
