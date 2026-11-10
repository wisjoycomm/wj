package main

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCode keeps every session in one SQLite file, opencode.db, next to a -wal file
// that holds the newest rows. The scanner opens it read-only, which sees the -wal too
// (the immutable mode does not, and would miss the latest calls).
//
// What is read, and only that:
//   session_message  type = 'assistant', with data.tokens, data.model and data.finish
//   session_v2       the session's directory (the project) and title
// No message text is ever read.

// openCodeDB is the path of opencode.db: $OPENCODE_DATA_DIR/opencode.db, else the
// XDG data folder OpenCode itself uses (~/.local/share/opencode on every system).
func openCodeDB(home string) string {
	dir := os.Getenv("OPENCODE_DATA_DIR")
	if dir == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "opencode")
	}
	return filepath.Join(dir, "opencode.db")
}

// sqliteReadOnlyURI turns a file path into a read-only URI the driver accepts on
// every system, including a Windows drive letter and spaces.
func sqliteReadOnlyURI(path string) string {
	slash := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String() + "?mode=ro&_pragma=busy_timeout(3000)"
}

type openCodeTokens struct {
	Input, Output, Reasoning, CacheRead, CacheWrite int64
}

// scanOpenCode reads the calls recorded since the last scan. OpenCode rows are
// append-only and numbered by time, so each scan asks only for the rows newer than the
// newest one already read.
func (s *sessionScanner) scanOpenCode(since time.Time) {
	path := s.openCodeDB
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// The database changes whenever a call is added; its -wal file changes first.
	stamp := info.ModTime()
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.ModTime().After(stamp) {
		stamp = wal.ModTime()
	}
	if !stamp.After(s.openCodeSeen) && s.openCodeLoaded {
		return
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyURI(path))
	if err != nil {
		return
	}
	defer db.Close()

	from := s.openCodeNewest
	if !s.openCodeLoaded {
		from = since.UnixMilli() // first scan: only what the period needs
		if since.IsZero() {
			from = 0
		}
	}
	rows, err := db.Query(`
		select m.id, m.session_id, m.time_created,
		       coalesce(json_extract(m.data,'$.model.id'), ''),
		       coalesce(json_extract(m.data,'$.tokens.input'), 0),
		       coalesce(json_extract(m.data,'$.tokens.output'), 0),
		       coalesce(json_extract(m.data,'$.tokens.reasoning'), 0),
		       coalesce(json_extract(m.data,'$.tokens.cache.read'), 0),
		       coalesce(json_extract(m.data,'$.tokens.cache.write'), 0),
		       coalesce(s.directory, ''), coalesce(s.title, ''), coalesce(s.parent_id, '')
		from session_message m left join session_v2 s on s.id = m.session_id
		where m.type = 'assistant' and m.time_created > ?
		  and json_extract(m.data,'$.tokens.input') is not null
		order by m.time_created`, from)
	if err != nil {
		return
	}
	defer rows.Close()
	newest := s.openCodeNewest
	for rows.Next() {
		var id, session, model, dir, title, parent string
		var created int64
		var t openCodeTokens
		if rows.Scan(&id, &session, &created, &model, &t.Input, &t.Output, &t.Reasoning, &t.CacheRead, &t.CacheWrite, &dir, &title, &parent) != nil {
			continue
		}
		newest = max(newest, created)
		if title != "" {
			s.titles[session] = title
		}
		// OpenCode counts reasoning apart from output; the other apps fold it into
		// output, so the columns mean the same thing for every app.
		call := sessionCall{
			When: time.UnixMilli(created), Project: s.projectOf(dir), App: "opencode", Model: model,
			Input: t.Input, Output: t.Output + t.Reasoning, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite,
			Session: session, Context: t.Input + t.CacheRead + t.CacheWrite,
		}
		if parent != "" { // a sub-session belongs to the session that started it
			call.Session, call.Sub, call.Agent = parent, true, shortID(session)
			if _, ok := s.agents[parent+"/"+call.Agent]; !ok {
				s.agents[parent+"/"+call.Agent] = agentMeta{Type: "opencode", Description: title}
			}
		}
		s.record("opencode\x00"+id, call)
	}
	s.openCodeNewest, s.openCodeSeen, s.openCodeLoaded = newest, stamp, true
}
