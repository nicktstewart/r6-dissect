package dissect

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"
)

type MatchUpdateType int

//go:generate stringer -type=MatchUpdateType
const (
	Kill MatchUpdateType = iota
	Death
	DefuserPlantStart
	DefuserPlantComplete
	DefuserDisableStart
	DefuserDisableComplete
	LocateObjective
	OperatorSwap
	Battleye
	PlayerLeave
	Other
)

type MatchUpdate struct {
	Type          MatchUpdateType `json:"type"`
	Username      string          `json:"username,omitempty"`
	Target        string          `json:"target,omitempty"`
	Headshot      *bool           `json:"headshot,omitempty"`
	Time          string          `json:"time"`
	TimeInSeconds float64         `json:"timeInSeconds"`
	Message       string          `json:"message,omitempty"`
	Operator      Operator        `json:"operator,omitempty"`
}

func (r *Reader) recordKillUpdate(u MatchUpdate) {
	killerIdx := r.PlayerIndexByUsername(u.Username)
	targetIdx := r.PlayerIndexByUsername(u.Target)
	if killerIdx >= 0 && targetIdx >= 0 &&
		r.Header.Players[killerIdx].TeamIndex == r.Header.Players[targetIdx].TeamIndex {
		return
	}
	for _, val := range r.MatchFeedback {
		if val.Type != Kill && val.Type != Death {
			continue
		}
		if val.Target == u.Target || (val.Type == Death && val.Username == u.Target) {
			return
		}
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
}

func (i MatchUpdateType) MarshalJSON() (text []byte, err error) {
	return json.Marshal(stringerIntMarshal{
		Name: i.String(),
		ID:   int(i),
	})
}

func (i *MatchUpdateType) UnmarshalJSON(data []byte) (err error) {
	var x stringerIntMarshal
	if err = json.Unmarshal(data, &x); err != nil {
		return
	}
	*i = MatchUpdateType(x.ID)
	return
}

var activity2 = []byte{0x00, 0x00, 0x00, 0x22, 0xe3, 0x09, 0x00, 0x79}
var killIndicator = []byte{0x22, 0xd9, 0x13, 0x3c, 0xba}

func readMatchFeedback(r *Reader) error {
	if r.Header.CodeVersion >= Y11S2 {
		return readY11S2MatchFeedback(r)
	}
	if r.Header.CodeVersion >= Y9S1Update3 {
		if err := r.Skip(38); err != nil {
			return err
		}
	} else if r.Header.CodeVersion >= Y9S1 {
		if err := r.Skip(9); err != nil {
			return err
		}
		valid, err := r.Int()
		if err != nil {
			return err
		}
		if valid != 4 {
			return errors.New("match feedback failed valid check")
		}
		if err := r.Skip(24); err != nil {
			return err
		}
	} else {
		if err := r.Skip(1); err != nil {
			return err
		}
		if err := r.Seek(activity2); err != nil {
			return err
		}
	}
	size, err := r.Int()
	if err != nil {
		return err
	}
	if size == 0 { // kill or an unknown indicator at start of match
		killTrace, err := r.Bytes(5)
		if err != nil {
			return err
		}
		if !bytes.Equal(killTrace, killIndicator) {
			log.Debug().Hex("killTrace", killTrace).Send()
			return nil
		}
		username, err := r.String()
		if err != nil {
			return err
		}
		empty := len(username) == 0
		if empty {
			log.Debug().Str("warn", "kill username empty").Send()
		}
		// No idea what these 15 bytes mean (kill type?)
		if err = r.Skip(15); err != nil {
			return err
		}
		target, err := r.String()
		if err != nil {
			return err
		}
		if empty && len(target) > 0 {
			u := MatchUpdate{
				Type:          Death,
				Username:      target,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
			log.Debug().Msg("kill username empty because of death")
			return nil
		} else if empty {
			return nil
		}
		u := MatchUpdate{
			Type:          Kill,
			Username:      username,
			Target:        target,
			Time:          r.timeRaw,
			TimeInSeconds: r.time,
		}
		if err = r.Skip(56); err != nil {
			return err
		}
		headshot, err := r.Int()
		if err != nil {
			return err
		}
		headshotPtr := new(bool)
		if headshot == 1 {
			*headshotPtr = true
		}
		u.Headshot = headshotPtr
		r.recordKillUpdate(u)
		return nil
	}
	// TODO: Y9S1 may have removed or modified other match feedback options
	if r.Header.CodeVersion >= Y9S1 {
		return nil
	}
	b, err := r.Bytes(size)
	if err != nil {
		return err
	}
	msg := string(b)
	t := Other
	if strings.Contains(msg, "bombs") || strings.Contains(msg, "objective") {
		t = LocateObjective
	}
	if strings.Contains(msg, "BattlEye") {
		t = Battleye
	}
	if strings.Contains(msg, "left") {
		t = PlayerLeave
	}
	username := strings.Split(msg, " ")[0]
	if t == Other {
		username = ""
	} else {
		msg = ""
	}
	u := MatchUpdate{
		Type:          t,
		Username:      username,
		Target:        "",
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
		Message:       msg,
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
	return nil
}

func readY11S2MatchFeedback(r *Reader) error {
	packetStart := r.offset - len([]byte{0x59, 0x34, 0xE5, 0x8B, 0x04})
	if packetStart < 0 {
		packetStart = 0
	}
	packetEnd := packetStart + 256
	if packetEnd > len(r.b) {
		packetEnd = len(r.b)
	}
	nextPacket := bytes.Index(r.b[r.offset:packetEnd], []byte{0x59, 0x34, 0xE5, 0x8B, 0x04})
	if nextPacket >= 0 {
		packetEnd = r.offset + nextPacket
	}

	indicatorOffset := bytes.Index(r.b[r.offset:packetEnd], killIndicator)
	if indicatorOffset < 0 {
		return nil
	}
	payloadStart := r.offset + indicatorOffset + len(killIndicator)
	payloadEnd := payloadStart + 160
	if payloadEnd > packetEnd {
		payloadEnd = packetEnd
	}

	names := r.y11s2FeedbackNames(payloadStart, payloadEnd)
	if len(names) == 1 {
		r.recordY11S2Death(names[0])
		return nil
	}
	if len(names) < 2 {
		return nil
	}
	r.recordY11S2Kill(names[0], names[1])
	return nil
}

func readY11S2KillIndicatorFeedback(r *Reader) error {
	payloadStart := r.offset
	payloadEnd := payloadStart + 160
	if payloadEnd > len(r.b) {
		payloadEnd = len(r.b)
	}

	names := r.y11s2FeedbackNames(payloadStart, payloadEnd)
	if len(names) == 1 {
		r.recordY11S2Death(names[0])
		return nil
	}
	if len(names) < 2 {
		return nil
	}
	r.recordY11S2Kill(names[0], names[1])
	return nil
}

func (r *Reader) recordY11S2Death(username string) {
	u := MatchUpdate{
		Type:          Death,
		Username:      username,
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
	}
	r.recordDeathUpdate(u)
}

func (r *Reader) recordY11S2Kill(username string, target string) {
	headshot := false
	u := MatchUpdate{
		Type:          Kill,
		Username:      username,
		Target:        target,
		Headshot:      &headshot,
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
	}
	r.recordKillUpdate(u)
}

func (r *Reader) recordDeathUpdate(u MatchUpdate) {
	for _, val := range r.MatchFeedback {
		switch val.Type {
		case Kill:
			if val.Target == u.Username {
				return
			}
		case Death:
			if val.Username == u.Username {
				return
			}
		}
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
}

func (r *Reader) y11s2FeedbackNames(start int, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > len(r.b) {
		end = len(r.b)
	}
	type nameHit struct {
		offset int
		name   string
	}
	hits := make([]nameHit, 0, 2)
	seen := make(map[string]struct{})
	for _, player := range r.Header.Players {
		name := player.Username
		if name == "" || len(name) > 255 {
			continue
		}
		raw := []byte(name)
		searchStart := start
		for searchStart+len(raw) <= end {
			relative := bytes.Index(r.b[searchStart:end], raw)
			if relative < 0 {
				break
			}
			offset := searchStart + relative
			searchStart = offset + 1
			if offset == 0 || int(r.b[offset-1]) != len(raw) {
				continue
			}
			if _, found := seen[name]; found {
				continue
			}
			seen[name] = struct{}{}
			hits = append(hits, nameHit{offset: offset, name: name})
			break
		}
	}
	if len(hits) == 0 {
		return nil
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].offset < hits[j-1].offset; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	if len(hits) == 1 {
		return []string{hits[0].name}
	}
	return []string{hits[0].name, hits[1].name}
}
