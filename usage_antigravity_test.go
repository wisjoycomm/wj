package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// pb builds protobuf bytes for the fixtures.
type pb []byte

func pbVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func (p pb) num(field, v uint64) pb { return pbVarint(pbVarint(p, field<<3), v) }

func (p pb) msg(field uint64, body []byte) pb {
	p = pbVarint(pbVarint(p, field<<3|2), uint64(len(body)))
	return append(p, body...)
}

func (p pb) str(field uint64, s string) pb { return p.msg(field, []byte(s)) }

func pbTime(t time.Time) pb { return pb{}.num(1, uint64(t.Unix())) }

type agFixture struct {
	t  *testing.T
	db *sql.DB
}

func newAgDB(t *testing.T, path string) *agFixture {
	t.Helper()
	writeTestFile(t, path, "")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		"create table steps (idx integer primary key, metadata blob)",
		"create table gen_metadata (idx integer primary key, data blob)",
		"create table trajectory_metadata_blob (id text primary key, data blob)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return &agFixture{t, db}
}

func (f *agFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

func agUsageBlob(modelID, input, output, cacheRead uint64, response string) pb {
	u := pb{}.num(1, modelID).num(2, input).num(3, output).num(5, cacheRead)
	if response != "" {
		u = u.str(11, response)
	}
	return u
}

func TestAntigravityReadsStepsAndGenerationsOnce(t *testing.T) {
	s, home := testScanner(t)
	t.Setenv("ANTIGRAVITY_DATA_DIR", "")
	when := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	dir := filepath.Join(home, ".gemini", "antigravity-cli", "conversations")
	f := newAgDB(t, filepath.Join(dir, "conv-1.db"))
	f.exec("insert into trajectory_metadata_blob values ('main', ?)", []byte(pb{}.msg(2, pbTime(when)).str(7, "file:///D:/w/game")))
	// A step with its usage, and the generation that holds the same call.
	f.exec("insert into steps values (0, ?)", []byte(pb{}.msg(9, agUsageBlob(1417, 1000, 50, 4000, "r1")).msg(8, pbTime(when.Add(time.Minute)))))
	gen := pb{}.msg(1, pb{}.msg(4, agUsageBlob(1417, 1000, 50, 4000, "r1")))
	f.exec("insert into gen_metadata values (0, ?)", []byte(gen))
	// A generation without an id, linked to step 3, and a failed attempt that copies it.
	usage := agUsageBlob(0, 200, 10, 0, "")
	gen2 := pb{}.msg(1, pb{}.str(19, "Gemini 3.8 Flash (High)").msg(4, usage).msg(17, pb{}.msg(2, usage))).num(2, 3)
	f.exec("insert into gen_metadata values (1, ?)", []byte(gen2))

	s.antigravityRoots = antigravityRoots(home)
	s.scan(time.Time{})
	if len(s.calls) != 2 {
		t.Fatalf("calls = %+v", s.calls)
	}
	a, b := s.calls[0], s.calls[1]
	if a.App != "antigravity" || a.Model != "gemini-4-argon" || a.Input != 1000 || a.CacheRead != 4000 || a.Project != "game" || !a.When.Equal(when.Add(time.Minute)) {
		t.Errorf("first call = %+v", a)
	}
	if b.Model != "gemini-3.8-flash-high" || b.Input != 200 || !b.When.Equal(when) || b.Session != "conv-1" {
		t.Errorf("second call = %+v", b)
	}
	if _, ok := priceOf(b.Model); !ok {
		t.Errorf("a thinking level bills as the plain model")
	}
	// Nothing changed: a second scan adds nothing.
	s.scan(time.Time{})
	if len(s.calls) != 2 {
		t.Errorf("rescan added calls: %d", len(s.calls))
	}
}

func TestAntigravitySubagentCountsUnderItsParent(t *testing.T) {
	s, home := testScanner(t)
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_DATA_DIR", dir)
	when := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	child := newAgDB(t, filepath.Join(dir, "child.db"))
	meta := pb{}.msg(2, pbTime(when)).str(5, "parent").str(7, "file:///D:/w/game").msg(8, pb{}.str(1, "image-generator").str(2, "Draw a sword"))
	child.exec("insert into trajectory_metadata_blob values ('main', ?)", []byte(meta))
	child.exec("insert into gen_metadata values (0, ?)", []byte(pb{}.msg(1, pb{}.msg(4, agUsageBlob(1318, 300, 20, 0, "c1")))))
	s.antigravityRoots = antigravityRoots(home)
	s.scan(time.Time{})
	if len(s.calls) != 1 {
		t.Fatalf("calls = %+v", s.calls)
	}
	c := s.calls[0]
	if c.Session != "parent" || !c.Sub || c.Agent != "child" || s.agents["parent/child"].Description != "Draw a sword" {
		t.Errorf("sub call = %+v, agents = %+v", c, s.agents)
	}
}

func TestAntigravityModelNames(t *testing.T) {
	for in, want := range map[string]string{
		"Gemini 3.8 Flash (High)": "gemini-3.8-flash-high",
		"Gemini 3.7 Pro Thinking": "gemini-3.7-pro",
		"model_placeholder_m402":  "claude-opus-5-5",
		"gemini-3-pro-low":        "gemini-3-pro",
		"":                        antigravityDefaultModel,
		"Something Else":          "Something Else",
	} {
		if got := normalizeAntigravityModel(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
	if antigravityModelName(1050) != "gemini-3.1-flash-lite" || antigravityModelName(1322) != "model_placeholder_m322" {
		t.Errorf("model ids")
	}
	for _, m := range []string{"gemini-3.6-flash-low", "claude-4.5-sonnet", "gemini-3.1-pro"} {
		if _, ok := priceOf(m); !ok {
			t.Errorf("%s has no price", m)
		}
	}
}

func TestClaudeDataDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := claudeDataDirs(home); len(got) != 1 || got[0] != filepath.Join(home, ".claude") {
		t.Errorf("default = %v", got)
	}
	writeTestFile(t, filepath.Join(home, ".config", "claude", "projects", "p", "a.jsonl"), "")
	if got := claudeDataDirs(home); len(got) != 2 || got[0] != filepath.Join(home, ".config", "claude") {
		t.Errorf("with ~/.config/claude = %v", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/a, /b/projects ,/a")
	if got := claudeDataDirs(home); len(got) != 2 || got[0] != filepath.Clean("/a") || got[1] != filepath.Clean("/b") {
		t.Errorf("from the variable = %v", got)
	}
}
