package dissect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestY11S2HeaderPlayersFromReplayHeader(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "test-files", "Match-2026-07-06_*", "*.rec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no Match-2026-07-06 replay files found in test-files")
	}

	for _, replay := range matches {
		t.Run(filepath.Base(replay), func(t *testing.T) {
			f, err := os.Open(replay)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			r, err := NewReader(f)
			if !Ok(err) {
				t.Fatalf("NewReader() error = %v", err)
			}
			if r == nil {
				t.Fatal("NewReader() returned nil reader")
			}
			if got, want := len(r.Header.Players), 10; got != want {
				t.Fatalf("header player count = %d, want %d", got, want)
			}
			for i, player := range r.Header.Players {
				if player.Username == "" {
					t.Fatalf("Header.Players[%d].Username is empty", i)
				}
				if player.RoleName != "" && player.Operator == 0 {
					t.Fatalf("Header.Players[%d].Operator is zero for roleName %q", i, player.RoleName)
				}
			}
		})
	}
}

func TestY11S2HeaderOperatorsFromRoleMarkers(t *testing.T) {
	replay := filepath.Join("..", "test-files", "Match-2026-07-06_20-13-55-24040", "Match-2026-07-06_20-13-55-24040-R01.rec")
	f, err := os.Open(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r, err := NewReader(f)
	if !Ok(err) {
		t.Fatalf("NewReader() error = %v", err)
	}

	want := map[string]Operator{
		"Ch4inon":         Azami,
		"H031PON":         Valkyrie,
		"smoking_man_T.K": Smoke,
		"Kyou.TARO":       Alibi,
		"AziStyle":        Ela,
		"Mori.CU":         Ace,
		"yoshiko.BME":     Twitch,
		"shiro_the_neko":  Blackbeard,
		"Otyakururu_":     Flores,
		"NotS.ujufdn.LV":  Nomad,
	}
	for _, player := range r.Header.Players {
		if got, ok := want[player.Username]; ok && player.Operator != got {
			t.Fatalf("%s operator = %v, want %v", player.Username, player.Operator, got)
		}
	}

	if err := r.Read(); !Ok(err) {
		t.Fatalf("Read() error = %v", err)
	}
	for _, player := range r.Header.Players {
		if got, ok := want[player.Username]; ok && player.Operator != got {
			t.Fatalf("%s operator after Read() = %v, want %v", player.Username, player.Operator, got)
		}
	}
}

func TestY11S2MatchFeedbackFromReplayEvents(t *testing.T) {
	replay := filepath.Join("..", "test-files", "Match-2026-07-06_20-13-55-24040", "Match-2026-07-06_20-13-55-24040-R01.rec")
	f, err := os.Open(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r, err := NewReader(f)
	if !Ok(err) {
		t.Fatalf("NewReader() error = %v", err)
	}
	if err := r.Read(); !Ok(err) {
		t.Fatalf("Read() error = %v", err)
	}
	if got, want := len(r.Header.Players), 10; got != want {
		t.Fatalf("player count after Read() = %d, want %d", got, want)
	}

	kills := r.KillsAndDeaths()
	if len(kills) == 0 {
		t.Fatal("expected Y11S2 kill/death match feedback, got none")
	}
	if len(kills) > 10 {
		t.Fatalf("Y11S2 kill/death feedback count = %d, want plausible 5v5 count", len(kills))
	}
	for _, update := range kills {
		if update.Type != Kill {
			t.Fatalf("unexpected feedback type %v in kill/death list", update.Type)
		}
		killerIdx := r.PlayerIndexByUsername(update.Username)
		targetIdx := r.PlayerIndexByUsername(update.Target)
		if killerIdx < 0 || targetIdx < 0 {
			t.Fatalf("unresolved kill pair: %q -> %q", update.Username, update.Target)
		}
		if r.Header.Players[killerIdx].TeamIndex == r.Header.Players[targetIdx].TeamIndex {
			t.Fatalf("same-team kill pair parsed: %q -> %q", update.Username, update.Target)
		}
	}
}

func TestY11S2KillIndicatorRecordsDeathOnlyPayload(t *testing.T) {
	r := &Reader{
		b:       append(append([]byte{}, killIndicator...), byte(len("target-player"))),
		offset:  len(killIndicator),
		time:    42,
		timeRaw: "0:42",
		Header: Header{
			CodeVersion: Y11S2,
			Players: []Player{
				{Username: "target-player", TeamIndex: 0},
				{Username: "other-player", TeamIndex: 1},
			},
		},
	}
	r.b = append(r.b, []byte("target-player")...)

	if err := readY11S2KillIndicatorFeedback(r); err != nil {
		t.Fatal(err)
	}
	if got, want := len(r.MatchFeedback), 1; got != want {
		t.Fatalf("match feedback count = %d, want %d", got, want)
	}
	update := r.MatchFeedback[0]
	if update.Type != Death || update.Username != "target-player" || update.Target != "" {
		t.Fatalf("death-only update = %#v", update)
	}
}

func TestY11S2DeathOnlyPayloadDoesNotDuplicateExistingKill(t *testing.T) {
	r := &Reader{
		b:       append(append([]byte{}, killIndicator...), byte(len("target-player"))),
		offset:  len(killIndicator),
		time:    42,
		timeRaw: "0:42",
		Header: Header{
			CodeVersion: Y11S2,
			Players: []Player{
				{Username: "killer-player", TeamIndex: 0},
				{Username: "target-player", TeamIndex: 1},
			},
		},
		MatchFeedback: []MatchUpdate{
			{Type: Kill, Username: "killer-player", Target: "target-player"},
		},
	}
	r.b = append(r.b, []byte("target-player")...)

	if err := readY11S2KillIndicatorFeedback(r); err != nil {
		t.Fatal(err)
	}
	if got, want := len(r.MatchFeedback), 1; got != want {
		t.Fatalf("match feedback count = %d, want %d", got, want)
	}
	if r.MatchFeedback[0].Type != Kill {
		t.Fatalf("existing kill was changed: %#v", r.MatchFeedback[0])
	}
}

func TestY11S2DefuserAttributionFromTimerRefs(t *testing.T) {
	tests := []struct {
		name  string
		match string
		round int
		want  []MatchUpdate
	}{
		{
			name:  "round 4 plant and disable",
			match: "Match-2026-07-06_20-13-55-24040",
			round: 3,
			want: []MatchUpdate{
				{Type: DefuserPlantStart, Username: "Mori.CU"},
				{Type: DefuserPlantComplete, Username: "Mori.CU"},
				{Type: DefuserDisableStart, Username: "H031PON"},
				{Type: DefuserDisableComplete, Username: "H031PON"},
			},
		},
		{
			name:  "round 8 interrupted plant",
			match: "Match-2026-07-06_20-13-55-24040",
			round: 7,
			want: []MatchUpdate{
				{Type: DefuserPlantStart, Username: "AziStyle"},
			},
		},
		{
			name:  "round 9 interrupted then completed plant",
			match: "Match-2026-07-06_20-13-55-24040",
			round: 8,
			want: []MatchUpdate{
				{Type: DefuserPlantStart, Username: "smoking_man_T.K"},
				{Type: DefuserPlantStart, Username: "Kyou.TARO"},
				{Type: DefuserPlantComplete, Username: "Kyou.TARO"},
			},
		},
		{
			name:  "round 12 repeated interrupted plants",
			match: "Match-2026-07-06_20-13-55-24040",
			round: 11,
			want: []MatchUpdate{
				{Type: DefuserPlantStart, Username: "H031PON"},
				{Type: DefuserPlantStart, Username: "H031PON"},
				{Type: DefuserPlantStart, Username: "H031PON"},
			},
		},
		{
			name:  "clubhouse round 9 defuser pickup before plant complete",
			match: "Match-2026-07-06_22-15-05-24040",
			round: 8,
			want: []MatchUpdate{
				{Type: DefuserPlantStart, Username: "Ch4inon"},
				{Type: DefuserPlantStart, Username: "AziStyle"},
				{Type: DefuserPlantComplete, Username: "AziStyle"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("..", "test-files", tt.match))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			m, err := NewMatchReader(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Read(); !Ok(err) {
				t.Fatal(err)
			}

			r, err := m.RoundAt(tt.round)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]MatchUpdate, 0)
			for _, update := range r.MatchFeedback {
				if update.Type == DefuserPlantStart ||
					update.Type == DefuserPlantComplete ||
					update.Type == DefuserDisableStart ||
					update.Type == DefuserDisableComplete {
					got = append(got, MatchUpdate{Type: update.Type, Username: update.Username})
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("defuser updates = %#v, want %#v", got, tt.want)
			}
			for i := range tt.want {
				if got[i].Type != tt.want[i].Type || got[i].Username != tt.want[i].Username {
					t.Fatalf("defuser update %d = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatchReaderContinuesAfterRoundTrailerEOF(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "test-files", "Match-2026-07-06_20-13-55-24040"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	m, err := NewMatchReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Read(); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(m.rounds) == 0 {
		t.Fatal("Read() returned zero rounds")
	}
	for i, round := range m.rounds {
		if round == nil {
			t.Fatalf("round %d was not read", i)
		}
		if got, want := len(round.Header.Players), 10; got != want {
			t.Fatalf("round %d player count = %d, want %d", i, got, want)
		}
	}
}

func TestMatchReaderKeepsSingleRoundMatch(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "test-files", "Match-2026-07-06_21-16-49-24040"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	m, err := NewMatchReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Read(); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got, want := len(m.rounds), 1; got != want {
		t.Fatalf("round count = %d, want %d", got, want)
	}
	if got, want := len(m.rounds[0].Header.Players), 10; got != want {
		t.Fatalf("player count = %d, want %d", got, want)
	}
}
