package main

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The Claude Code and Codex session files can add up to gigabytes, and they are deleted
// after a while. What wj read from them is kept in one file under ~/.wj: the next run
// reads only the bytes appended since, and calls of a deleted file are still reported.
// Only numbers and metadata are kept, never the text of a prompt or reply.

const (
	usageStoreName    = "usage-store.gob"
	usageStoreVersion = 1
	// ponytail: the whole store is rewritten on save, so a scan every few seconds saves
	// at most this often; split it per month if it grows too big to rewrite.
	usageStoreEvery = 30 * time.Second
)

type usageStore struct {
	Version     int
	Files       map[string]storedFile
	Calls       []storedCall
	Projects    map[string]string
	Titles      map[string]string
	Agents      map[string]agentMeta
	Compactions map[string]compaction
}

// storedFile is a sessionFile: how far it was read and what a later line needs to know.
type storedFile struct {
	Offset, LastCtx int64
	ModTime         time.Time
	Cwd, Model      string
	HasRecords      bool
	LastTotal       codexTokens
	Tools           []string
}

type storedCall struct {
	Key, File string
	Call      sessionCall
}

// fileRoots are the folders that hold the JSONL session files.
func (s *sessionScanner) fileRoots() []string {
	var roots []string
	for _, dir := range s.claudeDirs {
		roots = append(roots, filepath.Join(dir, "projects"))
	}
	for _, dir := range []string{"sessions", "archived_sessions"} {
		roots = append(roots, filepath.Join(s.codexDir, dir))
	}
	return roots
}

// loadStore runs once per scanner. A missing, broken or old store is ignored: the
// session files are then read from the start and the store is written again.
func (s *sessionScanner) loadStore() {
	if s.storeLoaded {
		return
	}
	s.storeLoaded = true
	f, err := os.Open(s.storePath)
	if err != nil {
		return
	}
	defer f.Close()
	var st usageStore
	if gob.NewDecoder(bufio.NewReader(f)).Decode(&st) != nil || st.Version != usageStoreVersion {
		s.storeDirty = true
		return
	}
	for path, f := range st.Files {
		s.files[path] = &sessionFile{offset: f.Offset, lastCtx: f.LastCtx, modTime: f.ModTime, cwd: f.Cwd,
			model: f.Model, hasRecords: f.HasRecords, lastTotal: f.LastTotal, tools: f.Tools}
	}
	for _, c := range st.Calls {
		s.owner[c.Key] = c.File
		s.record(c.Key, c.Call)
	}
	for k, v := range st.Projects {
		s.projects[k] = v
	}
	for k, v := range st.Titles {
		s.titles[k] = v
	}
	for k, v := range st.Agents {
		s.agents[k] = v
	}
	for k, v := range st.Compactions {
		s.compactions[k] = v
	}
}

// saveStore writes the store when something new was read. Failing to write it only
// means the next run reads more; the report is not affected.
func (s *sessionScanner) saveStore() {
	if !s.storeDirty || time.Since(s.storeSaved) < usageStoreEvery {
		return
	}
	if err := s.writeStore(); err == nil {
		s.storeDirty, s.storeSaved = false, time.Now()
	}
}

func (s *sessionScanner) writeStore() error {
	st := usageStore{Version: usageStoreVersion, Files: map[string]storedFile{}, Projects: s.projects,
		Titles: s.titles, Agents: s.agents, Compactions: s.compactions}
	for path, f := range s.files {
		st.Files[path] = storedFile{Offset: f.offset, LastCtx: f.lastCtx, ModTime: f.modTime, Cwd: f.cwd,
			Model: f.model, HasRecords: f.hasRecords, LastTotal: f.lastTotal, Tools: f.tools}
	}
	for key, i := range s.index {
		// OpenCode and Antigravity calls come from SQLite, which is read again each run.
		if strings.HasPrefix(key, "claude\x00") || strings.HasPrefix(key, "codex\x00") {
			st.Calls = append(st.Calls, storedCall{Key: key, File: s.owner[key], Call: s.calls[i]})
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o700); err != nil {
		return fmt.Errorf("usage store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.storePath), ".usage-store-*")
	if err != nil {
		return fmt.Errorf("usage store: %w", err)
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriter(tmp)
	if err = gob.NewEncoder(w).Encode(st); err == nil {
		err = w.Flush()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("usage store: %w", err)
	}
	return os.Rename(tmp.Name(), s.storePath)
}

// dropCallsOf forgets the calls a file recorded, before that file is read again from
// the start, so they are not counted twice.
// ponytail: a call copied into two files is owned by the last one read; if that file is
// replaced by other content the copy in the first file is lost until its own re-read.
func (s *sessionScanner) dropCallsOf(path string) {
	keyOf := make([]string, len(s.calls))
	for key, i := range s.index {
		keyOf[i] = key
	}
	calls := s.calls[:0]
	for i, call := range s.calls {
		key := keyOf[i]
		if key != "" && s.owner[key] == path {
			delete(s.index, key)
			delete(s.owner, key)
			continue
		}
		if key != "" {
			s.index[key] = len(calls)
		}
		calls = append(calls, call)
	}
	s.calls = calls
	s.storeDirty = true
}
