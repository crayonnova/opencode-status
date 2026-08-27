package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestUpsertAndQuery(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertModel("minimax/minimax-m3:free", "minimax", "MiniMax M3 (free)", true); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertModel("anthropic/claude-sonnet-4-5", "anthropic", "Sonnet 4.5", false); err != nil {
		t.Fatal(err)
	}
	all, err := s.AllModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	// Free models first.
	if !all[0].IsFree {
		t.Errorf("expected first row to be free, got %+v", all[0])
	}
}

func TestUptimeFraction(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertModel("a/free:free", "a", "A Free", true); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	checks := []struct {
		available bool
		at        time.Time
	}{
		{true, now.Add(-30 * time.Minute)},
		{true, now.Add(-20 * time.Minute)},
		{false, now.Add(-10 * time.Minute)},
		{true, now},
	}
	for _, c := range checks {
		if err := s.RecordCheck("a/free:free", c.available, c.at); err != nil {
			t.Fatal(err)
		}
	}
	f, n, err := s.UptimeFraction("a/free:free", now.Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("expected 4 samples, got %d", n)
	}
	if f != 0.75 {
		t.Errorf("expected 0.75, got %f", f)
	}
}

func TestPrune(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertModel("a", "a", "A", true); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now()
	_ = s.RecordCheck("a", true, old)
	_ = s.RecordCheck("a", true, old.Add(time.Hour))
	_ = s.RecordCheck("a", false, recent)
	deleted, err := s.Prune(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}
}

func TestHistoryOrdered(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpsertModel("a", "a", "A", true); err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	for i := 0; i < 5; i++ {
		_ = s.RecordCheck("a", i%2 == 0, base.Add(time.Duration(i)*time.Minute))
	}
	hist, err := s.History("a", base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 5 {
		t.Fatalf("expected 5, got %d", len(hist))
	}
	for i := 1; i < len(hist); i++ {
		if hist[i].CheckedAt.Before(hist[i-1].CheckedAt) {
			t.Fatalf("history not ordered at %d", i)
		}
	}
}
