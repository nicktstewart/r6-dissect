package dissect

import "testing"

func TestPlayerStatsMissingScoreboardRows(t *testing.T) {
	headshot := true
	r := &Reader{
		Header: Header{
			Teams: [2]Team{
				{Name: "Blue", Won: true},
				{Name: "Orange"},
			},
			Players: []Player{
				{Username: "player-1", TeamIndex: 0},
				{Username: "player-2", TeamIndex: 1},
			},
		},
		MatchFeedback: []MatchUpdate{
			{Type: Kill, Username: "player-1", Target: "player-2", Headshot: &headshot},
			{Type: Death, Username: "unknown-player"},
			{Type: Kill, Username: "player-1", Target: "unknown-player"},
		},
	}

	stats := r.PlayerStats()
	if len(stats) != 2 {
		t.Fatalf("PlayerStats() returned %d players, want 2", len(stats))
	}
	if stats[0].Kills != 2 {
		t.Errorf("player-1 kills = %d, want 2", stats[0].Kills)
	}
	if stats[0].Headshots != 1 {
		t.Errorf("player-1 headshots = %d, want 1", stats[0].Headshots)
	}
	if !stats[1].Died {
		t.Error("player-2 died = false, want true")
	}
}

func TestMatchPlayerStatsReusesExistingPlayers(t *testing.T) {
	m := &MatchReader{
		rounds: []*Reader{
			{
				Header: Header{
					Players: []Player{
						{Username: "player-1", TeamIndex: 0},
						{Username: "player-2", TeamIndex: 1},
					},
				},
				MatchFeedback: []MatchUpdate{{Type: Death, Username: "player-2"}},
			},
			{
				Header: Header{
					Players: []Player{
						{Username: "player-2", TeamIndex: 1},
						{Username: "player-1", TeamIndex: 0},
					},
				},
				MatchFeedback: []MatchUpdate{{Type: Death, Username: "player-1"}},
			},
		},
	}

	stats := m.PlayerStats()
	if len(stats) != 2 {
		t.Fatalf("PlayerStats() returned %d match players, want 2", len(stats))
	}
	if stats[0].Username != "player-1" || stats[0].Rounds != 2 || stats[0].Deaths != 1 {
		t.Errorf("player-1 aggregate = %+v, want 2 rounds and 1 death", stats[0])
	}
	if stats[1].Username != "player-2" || stats[1].Rounds != 2 || stats[1].Deaths != 1 {
		t.Errorf("player-2 aggregate = %+v, want 2 rounds and 1 death", stats[1])
	}
}
