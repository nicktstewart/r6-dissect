package dissect

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

type Scoreboard struct {
	Players []ScoreboardPlayer
}

type ScoreboardPlayer struct {
	ID               []byte
	Kills            uint32
	Score            uint32
	Assists          uint32
	AssistsFromRound uint32
}

type scoreboardRowCell struct {
	kind      string
	offset    int
	value     uint32
	ref       uint64
	refValid  bool
	refOwner  string
	refAlias  bool
	markerLen int
}

type scoreboardRowBlock struct {
	start     int
	end       int
	cells     []scoreboardRowCell
	ownedRefs []uint64
	localRefs []uint64
}

const (
	scoreboardPatternLen        = 4
	scoreboardDebugWindowBefore = 32
	scoreboardDebugWindowAfter  = 96
)

var scoreboardDebugEnabled = os.Getenv("R6_SCOREBOARD_DEBUG") == "1"

func (r *Reader) scoreboardValidPlayerIndex(idx int) int {
	if idx < 0 || idx >= len(r.Header.Players) || idx >= len(r.Scoreboard.Players) {
		return -1
	}
	return idx
}

func (r *Reader) scoreboardValidHeaderPlayerIndex(idx int) int {
	if idx < 0 || idx >= len(r.Header.Players) {
		return -1
	}
	return idx
}

func (r *Reader) scoreboardPlayerIndexFromEntityRefs(start int, end int) int {
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
		playerIndex, found := r.playerEntityRefOwners[ref]
		if !found {
			playerIndex, found = r.playerTimerAliasRefOwners[ref]
		}
		if !found || r.scoreboardValidPlayerIndex(playerIndex) == -1 {
			continue
		}
		entry := candidates[playerIndex]
		entry.count++
		if entry.firstOffset == 0 || i < entry.firstOffset {
			entry.firstOffset = i
		}
		candidates[playerIndex] = entry
	}

	bestPlayerIndex := -1
	bestCount := 0
	bestOffset := end + 1
	for playerIndex, entry := range candidates {
		if entry.count > bestCount || (entry.count == bestCount && entry.firstOffset < bestOffset) {
			bestPlayerIndex = playerIndex
			bestCount = entry.count
			bestOffset = entry.firstOffset
		}
	}
	return bestPlayerIndex
}

func (r *Reader) scoreboardPlayerIndexFromUsernameWindow(start int, end int) int {
	bestPlayerIndex := -1
	bestOffset := end + 1
	for i, player := range r.Header.Players {
		if player.Username == "" {
			continue
		}
		offset := bytes.Index(r.b[start:end], []byte(player.Username))
		if offset == -1 {
			continue
		}
		offset += start
		if offset < bestOffset {
			bestOffset = offset
			bestPlayerIndex = i
		}
	}
	return bestPlayerIndex
}

func (r *Reader) scoreboardPlayerIndex(fixedIDOffset int) (int, []byte) {
	start := r.offset
	if err := r.Skip(fixedIDOffset); err == nil {
		if id, err := r.Bytes(4); err == nil {
			if idx := r.playerIndexByID(id); idx != -1 {
				return idx, id
			}
		}
	}

	if r.Header.CodeVersion < Y11S1 {
		return -1, nil
	}

	const scoreboardSearchWindow = 96
	end := start + scoreboardSearchWindow
	if end > len(r.b) {
		end = len(r.b)
	}
	if idx := r.scoreboardPlayerIndexFromUsernameWindow(start, end); idx != -1 {
		return idx, nil
	}
	if idx := r.scoreboardPlayerIndexFromEntityRefs(start, end); idx != -1 {
		return idx, nil
	}
	for pos := start; pos+4 <= end; pos++ {
		id := r.b[pos : pos+4]
		if idx := r.playerIndexByID(id); idx != -1 {
			return idx, id
		}
	}
	return -1, nil
}

func scoreboardWindowBounds(packetOffset int, b []byte) (int, int) {
	windowStart := packetOffset - scoreboardDebugWindowBefore
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := packetOffset + scoreboardDebugWindowAfter
	if windowEnd > len(b) {
		windowEnd = len(b)
	}
	return windowStart, windowEnd
}

func formatScoreboardWindow(packetOffset int, b []byte) string {
	windowStart, windowEnd := scoreboardWindowBounds(packetOffset, b)
	var builder strings.Builder
	for i := windowStart; i < windowEnd; i++ {
		if i > windowStart {
			builder.WriteByte(' ')
		}
		if i == packetOffset {
			builder.WriteByte('[')
		}
		builder.WriteString(strings.ToUpper(hex.EncodeToString(b[i : i+1])))
		if i == packetOffset+scoreboardPatternLen-1 {
			builder.WriteByte(']')
		}
	}
	return builder.String()
}

func (r *Reader) logScoreboardPlayerCorpus() {
	if !scoreboardDebugEnabled || r.Header.CodeVersion < Y11S1 {
		return
	}
	if r.scoreboardPlayerCorpusLogged {
		return
	}
	r.scoreboardPlayerCorpusLogged = true
	for i, player := range r.Header.Players {
		dissectID := strings.ToUpper(hex.EncodeToString(player.DissectID))
		usernameHex := strings.ToUpper(hex.EncodeToString([]byte(player.Username)))
		log.Debug().
			Int("player_index", i).
			Str("username", player.Username).
			Int("team_index", player.TeamIndex).
			Str("dissect_id", dissectID).
			Uint64("ui_id", player.uiID).
			Str("ui_low4", fmt.Sprintf("%08X", uint32(player.uiID))).
			Str("ui_high4", fmt.Sprintf("%08X", uint32(player.uiID>>32))).
			Str("profile_id", player.ProfileID).
			Str("username_hex", usernameHex).
			Msg("scoreboard_player_corpus")
	}
}

func (r *Reader) logScoreboardPacketFamily(family string, packetOffset int, value uint32, idx int, id []byte) {
	if !scoreboardDebugEnabled || r.Header.CodeVersion < Y11S1 {
		return
	}
	windowStart, windowEnd := scoreboardWindowBounds(packetOffset, r.b)
	event := log.Debug().
		Str("family", family).
		Int("packet_offset", packetOffset).
		Int("payload_offset", packetOffset+scoreboardPatternLen).
		Uint32("value", value).
		Str("window", formatScoreboardWindow(packetOffset, r.b))

	if len(id) > 0 {
		event = event.Str("resolved_id", strings.ToUpper(hex.EncodeToString(id)))
	}
	if idx >= 0 && idx < len(r.Header.Players) {
		player := r.Header.Players[idx]
		event = event.
			Int("player_index", idx).
			Str("username", player.Username).
			Str("player_dissect_id", strings.ToUpper(hex.EncodeToString(player.DissectID))).
			Uint64("player_ui_id", player.uiID).
			Str("player_ui_low4", fmt.Sprintf("%08X", uint32(player.uiID))).
			Str("player_ui_high4", fmt.Sprintf("%08X", uint32(player.uiID>>32)))
	} else {
		event = event.Str("username", "N/A")
	}

	usernameHits := make([]string, 0, 2)
	for _, player := range r.Header.Players {
		if player.Username == "" || !bytes.Contains(r.b[windowStart:windowEnd], []byte(player.Username)) {
			continue
		}
		usernameHits = append(usernameHits, player.Username)
	}
	if len(usernameHits) > 0 {
		event = event.Str("username_hits", strings.Join(usernameHits, ","))
	}

	refHits := make([]string, 0, 4)
	for i := windowStart; i+9 <= windowEnd; i++ {
		ref, ok := entityRefValueAt(r.b, i)
		if !ok {
			continue
		}
		if playerIndex, found := r.playerEntityRefOwners[ref]; found {
			if playerIndex = r.scoreboardValidHeaderPlayerIndex(playerIndex); playerIndex != -1 {
				refHits = append(refHits, fmt.Sprintf("entity:%s@%d:%s", refString(ref), i, r.Header.Players[playerIndex].Username))
			}
			continue
		}
		if playerIndex, found := r.playerTimerAliasRefOwners[ref]; found {
			if playerIndex = r.scoreboardValidHeaderPlayerIndex(playerIndex); playerIndex != -1 {
				refHits = append(refHits, fmt.Sprintf("alias:%s@%d:%s", refString(ref), i, r.Header.Players[playerIndex].Username))
			}
		}
	}
	if len(refHits) > 0 {
		event = event.Str("ref_hits", strings.Join(refHits, ","))
	}

	for pos := windowStart; pos+4 <= windowEnd; pos++ {
		idBytes := r.b[pos : pos+4]
		playerIndex := r.playerIndexByID(idBytes)
		if playerIndex == -1 {
			continue
		}
		event = event.Str(fmt.Sprintf("dissect_id_hit_%d", pos-windowStart), fmt.Sprintf("%d:%s", pos, r.Header.Players[playerIndex].Username))
	}
	for pos := windowStart; pos+8 <= windowEnd; pos++ {
		v := binary.LittleEndian.Uint64(r.b[pos : pos+8])
		for playerIndex, player := range r.Header.Players {
			if player.uiID == 0 || v != player.uiID {
				continue
			}
			event = event.Str(fmt.Sprintf("ui_id_hit_%d", pos-windowStart), fmt.Sprintf("%d:%s", pos, r.Header.Players[playerIndex].Username))
		}
	}
	event.Msg("scoreboard_packet_window")
}

func refString(ref uint64) string {
	return fmt.Sprintf("%016X", ref)
}

func scoreboardMarkerKindAt(b []byte, pos int) string {
	if pos < 0 || pos+4 > len(b) {
		return ""
	}
	switch {
	case bytes.Equal(b[pos:pos+4], []byte{0xEC, 0xDA, 0x4F, 0x80}):
		return "score"
	case bytes.Equal(b[pos:pos+4], []byte{0x1C, 0xD2, 0xB1, 0x9D}):
		return "kill"
	case bytes.Equal(b[pos:pos+4], []byte{0x4D, 0x73, 0x7F, 0x9E}):
		return "assist"
	default:
		return ""
	}
}

func (r *Reader) scoreboardOwnerLabel(ref uint64) (string, bool) {
	if owner, found := r.playerEntityRefOwners[ref]; found && owner >= 0 && owner < len(r.Header.Players) {
		return r.Header.Players[owner].Username, false
	}
	if owner, found := r.playerTimerAliasRefOwners[ref]; found && owner >= 0 && owner < len(r.Header.Players) {
		return r.Header.Players[owner].Username, true
	}
	return "", false
}

func (r *Reader) scoreboardCellAt(pos int) (scoreboardRowCell, bool) {
	kind := scoreboardMarkerKindAt(r.b, pos)
	if kind == "" || pos < 9 || pos+9 > len(r.b) || r.b[pos+4] != 0x04 {
		return scoreboardRowCell{}, false
	}
	cell := scoreboardRowCell{
		kind:      kind,
		offset:    pos,
		value:     binary.LittleEndian.Uint32(r.b[pos+5 : pos+9]),
		markerLen: 4,
	}
	if ref, ok := entityRefValueAt(r.b, pos-9); ok {
		cell.ref = ref
		cell.refValid = true
		cell.refOwner, cell.refAlias = r.scoreboardOwnerLabel(ref)
	}
	return cell, true
}

func (r *Reader) scoreboardCellsInWindow(start int, end int) []scoreboardRowCell {
	if start < 0 {
		start = 0
	}
	if end > len(r.b) {
		end = len(r.b)
	}
	cells := make([]scoreboardRowCell, 0, 16)
	for pos := start; pos+9 <= end; pos++ {
		cell, ok := r.scoreboardCellAt(pos)
		if !ok {
			continue
		}
		cells = append(cells, cell)
		pos += 3
	}
	return cells
}

func formatScoreboardRowCell(cell scoreboardRowCell) string {
	refLabel := "none"
	if cell.refValid {
		refLabel = refString(cell.ref)
		if cell.refOwner != "" {
			if cell.refAlias {
				refLabel = fmt.Sprintf("%s:alias:%s", refLabel, cell.refOwner)
			} else {
				refLabel = fmt.Sprintf("%s:%s", refLabel, cell.refOwner)
			}
		}
	}
	return fmt.Sprintf("%s@%d[%s]=%d", cell.kind, cell.offset, refLabel, cell.value)
}

func collectScoreboardRowRefs(b []byte, start int, end int) []uint64 {
	if start < 0 {
		start = 0
	}
	if end > len(b) {
		end = len(b)
	}
	refs := make([]uint64, 0, 8)
	seen := make(map[uint64]struct{})
	for pos := start; pos+9 <= end; pos++ {
		ref, ok := entityRefValueAt(b, pos)
		if !ok {
			continue
		}
		if _, found := seen[ref]; found {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func (r *Reader) scoreboardRowBlockAt(packetOffset int, gap int) (scoreboardRowBlock, bool) {
	if r.Header.CodeVersion < Y11S1 || packetOffset < 0 || packetOffset >= len(r.b) {
		return scoreboardRowBlock{}, false
	}
	windowStart := packetOffset - 192
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := packetOffset + 224
	if windowEnd > len(r.b) {
		windowEnd = len(r.b)
	}
	cells := r.scoreboardCellsInWindow(windowStart, windowEnd)
	target := -1
	for i, cell := range cells {
		if cell.offset == packetOffset {
			target = i
			break
		}
	}
	if target == -1 {
		return scoreboardRowBlock{}, false
	}
	blockStart := target
	for blockStart > 0 && cells[blockStart].offset-cells[blockStart-1].offset <= gap {
		blockStart--
	}
	blockEnd := target
	for blockEnd+1 < len(cells) && cells[blockEnd+1].offset-cells[blockEnd].offset <= gap {
		blockEnd++
	}
	if blockStart >= len(cells) || blockEnd < blockStart {
		return scoreboardRowBlock{}, false
	}
	blockWindowStart := cells[blockStart].offset - 32
	if blockWindowStart < 0 {
		blockWindowStart = 0
	}
	blockWindowEnd := cells[blockEnd].offset + 96
	if blockWindowEnd > len(r.b) {
		blockWindowEnd = len(r.b)
	}
	refs := collectScoreboardRowRefs(r.b, blockWindowStart, blockWindowEnd)
	block := scoreboardRowBlock{
		start: cells[blockStart].offset,
		end:   cells[blockEnd].offset,
		cells: append([]scoreboardRowCell(nil), cells[blockStart:blockEnd+1]...),
	}
	for _, ref := range refs {
		if owner, _ := r.scoreboardOwnerLabel(ref); owner != "" {
			block.ownedRefs = append(block.ownedRefs, ref)
		} else {
			block.localRefs = append(block.localRefs, ref)
		}
	}
	return block, true
}

func (r *Reader) scoreboardLateRowBlock(packetOffset int) (string, bool) {
	block, ok := r.scoreboardRowBlockAt(packetOffset, 96)
	if !ok {
		return "", false
	}
	ownedRefs := make([]string, 0, 4)
	localRefs := make([]string, 0, 8)
	for _, ref := range block.ownedRefs {
		refHex := refString(ref)
		if owner, alias := r.scoreboardOwnerLabel(ref); owner != "" {
			label := fmt.Sprintf("%s:%s", refHex, owner)
			if alias {
				label = fmt.Sprintf("%s:alias", label)
			}
			ownedRefs = append(ownedRefs, label)
		}
	}
	for _, ref := range block.localRefs {
		localRefs = append(localRefs, refString(ref))
	}
	shape := "late_row_block"
	if len(ownedRefs) == 0 {
		shape = "bootstrap_or_unanchored"
	}
	formattedCells := make([]string, 0, len(block.cells))
	for _, cell := range block.cells {
		formattedCells = append(formattedCells, formatScoreboardRowCell(cell))
	}
	return fmt.Sprintf(
		"shape=%s; block_offsets=%d-%d; cells=%s; owned_refs=%s; local_refs=%s",
		shape,
		block.start,
		block.end,
		strings.Join(formattedCells, ","),
		strings.Join(ownedRefs, ","),
		strings.Join(localRefs, ","),
	), true
}

func hasSharedRef(a []uint64, b []uint64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[uint64]struct{}, len(a))
	for _, ref := range a {
		set[ref] = struct{}{}
	}
	for _, ref := range b {
		if _, found := set[ref]; found {
			return true
		}
	}
	return false
}

func (r *Reader) scoreboardPlayerIndexFromWindow(start int, end int) int {
	if idx := r.scoreboardPlayerIndexFromEntityRefs(start, end); idx != -1 {
		return idx
	}
	if idx := r.scoreboardPlayerIndexFromUsernameWindow(start, end); idx != -1 {
		return r.scoreboardValidPlayerIndex(idx)
	}
	for pos := start; pos+4 <= end; pos++ {
		if idx := r.playerIndexByID(r.b[pos : pos+4]); idx != -1 {
			return r.scoreboardValidPlayerIndex(idx)
		}
	}
	return -1
}

func (r *Reader) scoreboardPlayerIndexFromRowCell(packetOffset int) int {
	cell, ok := r.scoreboardCellAt(packetOffset)
	if !ok || !cell.refValid {
		return -1
	}
	if playerIndex, found := r.playerEntityRefOwners[cell.ref]; found {
		return r.scoreboardValidPlayerIndex(playerIndex)
	}
	if playerIndex, found := r.playerTimerAliasRefOwners[cell.ref]; found {
		return r.scoreboardValidPlayerIndex(playerIndex)
	}
	return -1
}

func (r *Reader) scoreboardPlayerIndexFromLateRowBlock(packetOffset int) int {
	if r.Header.CodeVersion < Y11S1 {
		return -1
	}
	block, ok := r.scoreboardRowBlockAt(packetOffset, 96)
	if !ok || len(block.ownedRefs) == 0 {
		return -1
	}
	if idx := r.scoreboardPlayerIndexFromRowCell(packetOffset); idx != -1 {
		return idx
	}
	if idx := r.scoreboardPlayerIndexFromLinkedScoreBlock(packetOffset); idx != -1 {
		return idx
	}
	return -1
}

func (r *Reader) scoreboardPlayerIndexForY11Score(packetOffset int) (int, []byte) {
	if idx := r.scoreboardPlayerIndexFromRowCell(packetOffset); idx != -1 {
		return idx, nil
	}
	return r.scoreboardPlayerIndex(13)
}

func (r *Reader) scoreboardPlayerIndexForY11KillAssist(packetOffset int, fixedIDOffset int) (int, []byte) {
	return r.scoreboardPlayerIndex(fixedIDOffset)
}

func (r *Reader) scoreboardPlayerIndexFromLinkedScoreBlock(packetOffset int) int {
	if r.Header.CodeVersion < Y11S1 {
		return -1
	}
	block, ok := r.scoreboardRowBlockAt(packetOffset, 96)
	if !ok || len(block.localRefs) == 0 {
		return -1
	}
	searchStart := packetOffset - 4096
	if searchStart < 0 {
		searchStart = 0
	}
	searchEnd := packetOffset + 4096
	if searchEnd > len(r.b) {
		searchEnd = len(r.b)
	}
	candidateIndexes := make(map[int]struct{})
	for pos := searchStart; pos+9 <= searchEnd; pos++ {
		cell, ok := r.scoreboardCellAt(pos)
		if !ok || cell.kind != "score" {
			continue
		}
		scoreBlock, ok := r.scoreboardRowBlockAt(pos, 96)
		if !ok || !hasSharedRef(block.localRefs, scoreBlock.localRefs) {
			continue
		}
		windowStart := scoreBlock.start - 32
		if windowStart < 0 {
			windowStart = 0
		}
		windowEnd := scoreBlock.end + 128
		if windowEnd > len(r.b) {
			windowEnd = len(r.b)
		}
		if idx := r.scoreboardPlayerIndexFromWindow(windowStart, windowEnd); idx != -1 {
			candidateIndexes[idx] = struct{}{}
		}
		pos = scoreBlock.end
	}
	if len(candidateIndexes) != 1 {
		return -1
	}
	for idx := range candidateIndexes {
		return idx
	}
	return -1
}

func (r *Reader) logScoreboardRowBlock(family string, packetOffset int) {
	if !scoreboardDebugEnabled || r.Header.CodeVersion < Y11S1 {
		return
	}
	block, ok := r.scoreboardLateRowBlock(packetOffset)
	if !ok {
		return
	}
	log.Debug().
		Str("family", family).
		Int("packet_offset", packetOffset).
		Str("row_block", block).
		Msg("scoreboard_row_block")
}

func (r *Reader) scoreboardNearbyCells(packetOffset int) string {
	if packetOffset < 0 || packetOffset >= len(r.b) {
		return ""
	}
	start := packetOffset - 256
	if start < 0 {
		start = 0
	}
	end := packetOffset + 256
	if end > len(r.b) {
		end = len(r.b)
	}
	cells := make([]string, 0, 12)
	for pos := start + 9; pos+9 <= end; pos++ {
		if !bytes.Equal(r.b[pos:pos+4], []byte{0xEC, 0xDA, 0x4F, 0x80}) {
			continue
		}
		ref, ok := entityRefValueAt(r.b, pos-9)
		if !ok {
			continue
		}
		if pos+9 > len(r.b) || r.b[pos+4] != 0x04 {
			continue
		}
		value := binary.LittleEndian.Uint32(r.b[pos+5 : pos+9])
		label := refString(ref)
		if owner, found := r.playerEntityRefOwners[ref]; found {
			if owner = r.scoreboardValidHeaderPlayerIndex(owner); owner != -1 {
				label = fmt.Sprintf("%s:%s", label, r.Header.Players[owner].Username)
			}
		} else if owner, found := r.playerTimerAliasRefOwners[ref]; found {
			if owner = r.scoreboardValidHeaderPlayerIndex(owner); owner != -1 {
				label = fmt.Sprintf("%s:alias:%s", label, r.Header.Players[owner].Username)
			}
		}
		cells = append(cells, fmt.Sprintf("%d=%s:%d", pos, label, value))
	}
	return strings.Join(cells, ",")
}

func readScoreboardKills(r *Reader) error {
	packetOffset := r.offset - scoreboardPatternLen
	kills, err := r.Uint32()
	if err != nil {
		return err
	}
	r.logScoreboardPlayerCorpus()
	idx, id := r.scoreboardPlayerIndexForY11KillAssist(packetOffset, 30)
	idx = r.scoreboardValidPlayerIndex(idx)
	if idx != -1 {
		username := r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Kills = kills
		event := log.Debug().
			Str("username", username).
			Uint32("kills", kills).
			Hex("id", id)
		if scoreboardDebugEnabled && r.Header.CodeVersion >= Y11S1 {
			event = event.Str("nearby_cells", r.scoreboardNearbyCells(packetOffset))
		}
		event.Msg("scoreboard_kill")
	}
	r.logScoreboardPacketFamily("kill", packetOffset, kills, idx, id)
	r.logScoreboardRowBlock("kill", packetOffset)
	return nil
}

func readScoreboardAssists(r *Reader) error {
	packetOffset := r.offset - scoreboardPatternLen
	assists, err := r.Uint32()
	if err != nil {
		return err
	}
	r.logScoreboardPlayerCorpus()
	if assists == 0 {
		return nil
	}
	idx, id := r.scoreboardPlayerIndexForY11KillAssist(packetOffset, 30)
	idx = r.scoreboardValidPlayerIndex(idx)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Assists = assists
		r.Scoreboard.Players[idx].AssistsFromRound++
	}
	event := log.Debug().
		Uint32("assists", assists).
		Hex("id", id).
		Str("username", username)
	if scoreboardDebugEnabled && r.Header.CodeVersion >= Y11S1 {
		event = event.Str("nearby_cells", r.scoreboardNearbyCells(packetOffset))
	}
	event.Msg("scoreboard_assists")
	r.logScoreboardPacketFamily("assist", packetOffset, assists, idx, id)
	r.logScoreboardRowBlock("assist", packetOffset)
	return nil
}

func readScoreboardScore(r *Reader) error {
	packetOffset := r.offset - scoreboardPatternLen
	score, err := r.Uint32()
	if err != nil {
		return err
	}
	r.logScoreboardPlayerCorpus()
	if score == 0 {
		return nil
	}
	idx, id := r.scoreboardPlayerIndexForY11Score(packetOffset)
	idx = r.scoreboardValidPlayerIndex(idx)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		if r.Header.CodeVersion >= Y11S1 {
			if score > r.Scoreboard.Players[idx].Score {
				r.Scoreboard.Players[idx].Score = score
			}
		} else {
			r.Scoreboard.Players[idx].Score = score
		}
	}
	log.Debug().
		Uint32("score", score).
		Hex("id", id).
		Str("username", username).
		Msg("scoreboard_score")
	r.logScoreboardPacketFamily("score", packetOffset, score, idx, id)
	return nil
}
