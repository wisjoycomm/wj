package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openCodeFixture writes an opencode.db with the two tables wj reads and lets a test
// add sessions and messages to it.
type openCodeFixture struct {
	t    *testing.T
	path string
	db   *sql.DB
	n    int
}

func newOpenCodeFixture(t *testing.T, home string) *openCodeFixture {
	t.Helper()
	path := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		"create table session_v2 (id text primary key, project_id text, parent_id text, directory text, title text)",
		"create table session_message (id text primary key, session_id text, type text, seq integer, time_created integer, time_updated integer, data text)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return &openCodeFixture{t: t, path: path, db: db}
}

func (f *openCodeFixture) session(id, dir, title, parent string) {
	f.t.Helper()
	if _, err := f.db.Exec("insert into session_v2 (id, project_id, parent_id, directory, title) values (?,?,?,?,?)", id, "p", parent, dir, title); err != nil {
		f.t.Fatal(err)
	}
}

func (f *openCodeFixture) message(session, typ, data string, when time.Time) {
	f.t.Helper()
	f.n++
	if _, err := f.db.Exec("insert into session_message (id, session_id, type, seq, time_created, time_updated, data) values (?,?,?,?,?,?,?)",
		fmt.Sprintf("msg_%03d", f.n), session, typ, f.n, when.UnixMilli(), when.UnixMilli(), data); err != nil {
		f.t.Fatal(err)
	}
}

// call adds an assistant message with token counts.
func (f *openCodeFixture) call(session string, when time.Time, model string, input, output, reasoning, read, write int) {
	f.t.Helper()
	data := fmt.Sprintf(`{"model":{"id":%q,"providerID":"gateway"},"tokens":{"input":%d,"output":%d,"reasoning":%d,"cache":{"read":%d,"write":%d}},"cost":0,"finish":"stop"}`,
		model, input, output, reasoning, read, write)
	f.message(session, "assistant", data, when)
}

func TestOpenCodeCallsBecomeSessionCalls(t *testing.T) {
	s, home := testScanner(t)
	f := newOpenCodeFixture(t, home)
	project := filepath.Join(home, "work", "game")
	f.session("ses_main", project, "Fix the shop", "")
	day := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	f.call("ses_main", day, "cc/claude-opus-5-5", 1000, 200, 50, 8000, 400)
	f.call("ses_main", day.Add(time.Minute), "cx/gpt-6.1-sol", 500, 100, 0, 0, 0)
	// Not usage: a user message, and an assistant message that failed (no tokens).
	f.message("ses_main", "user", `{"text":"hello"}`, day)
	f.message("ses_main", "assistant", `{"model":{"id":"x"},"finish":"error"}`, day)
	s.scan(time.Time{})

	if len(s.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (only assistant messages with tokens)", len(s.calls))
	}
	c := s.calls[0]
	if c.App != "opencode" || c.Project != "game" || c.Model != "cc/claude-opus-5-5" || c.Session != "ses_main" || c.Sub {
		t.Errorf("call = %+v", c)
	}
	// Reasoning tokens join the output, so every app's columns mean the same thing.
	if c.Input != 1000 || c.Output != 250 || c.CacheRead != 8000 || c.CacheWrite != 400 || c.Context != 9400 {
		t.Errorf("tokens = %+v", c)
	}
	if !c.When.Equal(day) {
		t.Errorf("when = %v, want %v (milliseconds, not seconds)", c.When, day)
	}
	if s.titles["ses_main"] != "Fix the shop" {
		t.Errorf("titles = %v", s.titles)
	}
}

func TestOpenCodeSubSessionsBelongToTheirParent(t *testing.T) {
	s, home := testScanner(t)
	f := newOpenCodeFixture(t, home)
	f.session("ses_parent", filepath.Join(home, "p"), "Main chat", "")
	f.session("ses_child_abcdef", filepath.Join(home, "p"), "Find the config", "ses_parent")
	day := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	f.call("ses_parent", day, "m", 100, 0, 0, 0, 0)
	f.call("ses_child_abcdef", day, "m", 1000, 0, 0, 0, 0)
	s.scan(time.Time{})
	p, _ := resolvePeriod("2026-11-09", time.Date(2026, 11, 9, 12, 0, 0, 0, testZone))
	_, sessions, total := s.periodReport(p, "", "opencode")
	if len(sessions) != 1 || total.Total != 1100 {
		t.Fatalf("sessions = %d, total = %d: the child must not be a session of its own", len(sessions), total.Total)
	}
	se := sessions[0]
	if se.ID != "ses_parent" || se.Agent != 1000 || len(se.workers) != 1 {
		t.Errorf("session = %+v", se)
	}
	for _, w := range se.workers {
		if w.Description != "Find the config" || w.Total != 1000 {
			t.Errorf("worker = %+v", w)
		}
	}
}

func TestOpenCodeSeesNewCallsOnTheNextScanAndNeverDuplicates(t *testing.T) {
	s, home := testScanner(t)
	f := newOpenCodeFixture(t, home)
	f.session("ses_a", filepath.Join(home, "p"), "", "")
	day := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	f.call("ses_a", day, "m", 100, 0, 0, 0, 0)
	s.scan(time.Time{})
	s.scan(time.Time{}) // nothing new: the same call is not counted again
	if len(s.calls) != 1 {
		t.Fatalf("after two scans of one call: %d calls", len(s.calls))
	}
	// The database changes (its modification time moves) and a newer call appears.
	f.call("ses_a", day.Add(time.Hour), "m", 200, 0, 0, 0, 0)
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(f.path, future, future); err != nil {
		t.Fatal(err)
	}
	s.scan(time.Time{})
	if len(s.calls) != 2 {
		t.Fatalf("a new call must be picked up: %d calls", len(s.calls))
	}
	var sum int64
	for _, c := range s.calls {
		sum += c.Input
	}
	if sum != 300 {
		t.Errorf("sum = %d", sum)
	}
}

func TestOpenCodeFirstScanOnlyReadsTheRequestedPeriod(t *testing.T) {
	s, home := testScanner(t)
	f := newOpenCodeFixture(t, home)
	f.session("ses_a", filepath.Join(home, "p"), "", "")
	f.call("ses_a", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "m", 1, 0, 0, 0, 0)
	f.call("ses_a", time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), "m", 10, 0, 0, 0, 0)
	s.scan(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if len(s.calls) != 1 || s.calls[0].Input != 10 {
		t.Errorf("calls = %+v", s.calls)
	}
}

func TestOpenCodeMissingOrBrokenDatabaseIsIgnored(t *testing.T) {
	s, home := testScanner(t)
	s.scan(time.Time{}) // no file at all
	if len(s.calls) != 0 {
		t.Errorf("calls = %d", len(s.calls))
	}
	path := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	writeTestFile(t, path, "this is not a sqlite file")
	s.scan(time.Time{})
	if len(s.calls) != 0 {
		t.Errorf("a broken file must not add calls: %d", len(s.calls))
	}
	// A database from another version, without the tables wj reads, is also skipped.
	other := filepath.Join(home, "other", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(other))
	_, _ = db.Exec("create table unrelated (a text)")
	db.Close()
	t.Setenv("OPENCODE_DATA_DIR", filepath.Dir(other))
	s2 := newSessionScanner(home)
	s2.scan(time.Time{})
	if len(s2.calls) != 0 {
		t.Errorf("unknown schema must add nothing: %d", len(s2.calls))
	}
}

func TestOpenCodePathFollowsTheEnvironment(t *testing.T) {
	home := filepath.Join("h", "user")
	t.Setenv("OPENCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	if got, want := openCodeDB(home), filepath.Join(home, ".local", "share", "opencode", "opencode.db"); got != want {
		t.Errorf("default = %s, want %s", got, want)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join("x", "data"))
	if got, want := openCodeDB(home), filepath.Join("x", "data", "opencode", "opencode.db"); got != want {
		t.Errorf("xdg = %s, want %s", got, want)
	}
	t.Setenv("OPENCODE_DATA_DIR", "o")
	if got, want := openCodeDB(home), filepath.Join("o", "opencode.db"); got != want {
		t.Errorf("explicit = %s, want %s", got, want)
	}
}

func TestSQLiteReadOnlyURIHandlesWindowsPathsAndSpaces(t *testing.T) {
	got := sqliteReadOnlyURI(filepath.Join(t.TempDir(), "my data", "opencode.db"))
	if !strings.HasPrefix(got, "file:/") {
		t.Errorf("uri = %s", got)
	}
	if !strings.Contains(got, "mode=ro") || strings.Contains(got, " ") {
		t.Errorf("uri must be read-only and escaped: %s", got)
	}
}

func TestOpenCodeAppFilterAndTreeIncludeIt(t *testing.T) {
	s, home := testScanner(t)
	f := newOpenCodeFixture(t, home)
	f.session("ses_a", filepath.Join(home, "p"), "", "")
	f.call("ses_a", time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC), "cc/claude-opus-5-5", 100, 0, 0, 0, 0)
	writeTestFile(t, filepath.Join(home, ".claude", "projects", "p", "c.jsonl"), claudeLine("c1", "/w/game", "2026-11-09T04:00:00Z", 7, 0, 0, 0))
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, testZone)
	all, _ := reportText(s, usageView{Spec: "2026-11-09", Top: -1}, now, 0, time.Time{})
	only, _ := reportText(s, usageView{Spec: "2026-11-09", Top: -1, Client: "opencode"}, now, 0, time.Time{})
	if !strings.Contains(all, "Tổng: 2 lượt") || !strings.Contains(only, "Tổng: 1 lượt") || !strings.Contains(only, "app opencode") {
		t.Errorf("filter by app:\nall: %.140s\nonly: %.160s", all, only)
	}
	// The same model through the gateway or not is one model in the tree.
	if !strings.Contains(all, "opus-5-5") {
		t.Errorf("tree lacks the model")
	}
}
