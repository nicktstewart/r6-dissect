package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/redraskal/r6-dissect/dissect"
)

func openMatch(t *testing.T, name string) *dissect.MatchReader {
	t.Helper()
	root := filepath.Join("data", "replays", "defuser_detection", name)
	f, err := os.Open(root)
	if err != nil {
		t.Fatalf("open match %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("close match %s: %v", name, err)
		}
	})
	m, err := dissect.NewMatchReader(f)
	if err != nil {
		t.Fatalf("new match reader %s: %v", name, err)
	}
	if err := m.Read(); !dissect.Ok(err) {
		t.Fatalf("read match %s: %v", name, err)
	}
	return m
}

func findUpdateByType(r *dissect.Reader, updateType dissect.MatchUpdateType) (dissect.MatchUpdate, bool) {
	for _, update := range r.MatchFeedback {
		if update.Type == updateType {
			return update, true
		}
	}
	return dissect.MatchUpdate{}, false
}

func requireUpdateUsername(t *testing.T, r *dissect.Reader, updateType dissect.MatchUpdateType, want string) {
	t.Helper()
	update, ok := findUpdateByType(r, updateType)
	if !ok {
		t.Fatalf("missing update %s in round %d", updateType, r.Header.RoundNumber)
	}
	if update.Username != want {
		t.Fatalf("update %s username = %q, want %q", updateType, update.Username, want)
	}
}

func requireNoUpdate(t *testing.T, r *dissect.Reader, updateType dissect.MatchUpdateType) {
	t.Helper()
	if update, ok := findUpdateByType(r, updateType); ok {
		t.Fatalf("unexpected update %s for %q in round %d", updateType, update.Username, r.Header.RoundNumber)
	}
}

func TestDefuserDetectionRegression(t *testing.T) {
	t.Run("match2 and match3 agree after prep forfeit is dropped", func(t *testing.T) {
		match2 := openMatch(t, "match2")
		match3 := openMatch(t, "match3")

		if got, want := match2.NumRounds(), 4; got != want {
			t.Fatalf("match2 rounds = %d, want %d", got, want)
		}
		if got, want := match3.NumRounds(), 4; got != want {
			t.Fatalf("match3 rounds = %d, want %d", got, want)
		}

		for _, m := range []*dissect.MatchReader{match2, match3} {
			round1, err := m.RoundAt(0)
			if err != nil {
				t.Fatal(err)
			}
			round2, err := m.RoundAt(1)
			if err != nil {
				t.Fatal(err)
			}
			round3, err := m.RoundAt(2)
			if err != nil {
				t.Fatal(err)
			}
			round4, err := m.RoundAt(3)
			if err != nil {
				t.Fatal(err)
			}

			requireNoUpdate(t, round1, dissect.DefuserPlantComplete)
			requireUpdateUsername(t, round2, dissect.DefuserPlantComplete, "Mori.CU")
			requireNoUpdate(t, round2, dissect.DefuserDisableComplete)
			requireUpdateUsername(t, round3, dissect.DefuserPlantComplete, "Mori.CU")
			requireUpdateUsername(t, round3, dissect.DefuserDisableComplete, "MoriRecruit")
			requireUpdateUsername(t, round4, dissect.DefuserPlantComplete, "Mori.CU")
			requireUpdateUsername(t, round4, dissect.DefuserDisableComplete, "MoriRecruit")
		}
	})

	t.Run("match5 round 4 keeps planter and disabler distinct", func(t *testing.T) {
		match5 := openMatch(t, "match5")
		round, err := match5.RoundAt(3)
		if err != nil {
			t.Fatal(err)
		}
		if round.Header.RoundNumber != 3 {
			t.Fatalf("match5 round index 3 has round number %d, want 3", round.Header.RoundNumber)
		}
		requireUpdateUsername(t, round, dissect.DefuserPlantComplete, "OreoSenpai")
		requireUpdateUsername(t, round, dissect.DefuserDisableComplete, "ironsmithers")
	})

	t.Run("match4 chaotic rounds still recover completion actors", func(t *testing.T) {
		match4 := openMatch(t, "match4")

		round2, err := match4.RoundAt(1)
		if err != nil {
			t.Fatal(err)
		}
		if round2.Header.RoundNumber != 1 {
			t.Fatalf("match4 round index 1 has round number %d, want 1", round2.Header.RoundNumber)
		}
		requireUpdateUsername(t, round2, dissect.DefuserPlantComplete, "OreoSenpai")
		requireUpdateUsername(t, round2, dissect.DefuserDisableComplete, "Rival-_")

		round5, err := match4.RoundAt(4)
		if err != nil {
			t.Fatal(err)
		}
		if round5.Header.RoundNumber != 4 {
			t.Fatalf("match4 round index 4 has round number %d, want 4", round5.Header.RoundNumber)
		}
		requireUpdateUsername(t, round5, dissect.DefuserPlantComplete, "IceDogs2003")
		requireUpdateUsername(t, round5, dissect.DefuserDisableComplete, "OreoSenpai")
	})

	t.Run("match6 post-plant kill round keeps plant without disable", func(t *testing.T) {
		match6 := openMatch(t, "match6")
		round, err := match6.RoundAt(0)
		if err != nil {
			t.Fatal(err)
		}
		if round.Header.RoundNumber != 2 {
			t.Fatalf("match6 round index 0 has round number %d, want 2", round.Header.RoundNumber)
		}
		requireUpdateUsername(t, round, dissect.DefuserPlantComplete, "Daddy.Mozzie")
		requireNoUpdate(t, round, dissect.DefuserDisableComplete)
	})

	t.Run("match7 chaotic disable round keeps planter and disabler distinct", func(t *testing.T) {
		match7 := openMatch(t, "match7")
		round, err := match7.RoundAt(0)
		if err != nil {
			t.Fatal(err)
		}
		if round.Header.RoundNumber != 2 {
			t.Fatalf("match7 round index 0 has round number %d, want 2", round.Header.RoundNumber)
		}
		requireUpdateUsername(t, round, dissect.DefuserPlantComplete, "PercSpinner")
		requireUpdateUsername(t, round, dissect.DefuserDisableComplete, "LundyTrades")
	})
}

