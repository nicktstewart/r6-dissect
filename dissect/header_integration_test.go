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
