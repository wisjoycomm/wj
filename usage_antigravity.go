package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Antigravity (the IDE and the agy CLI) keeps one SQLite file per conversation in
// ~/.gemini/antigravity*/conversations/<id>.db. The usage of each model call is a
// protobuf blob; this reader follows the field numbers ccusage found
// (github.com/ccusage/ccusage, rust/adapters/antigravity). What is read, and only that:
//
//	steps.metadata             one step: usage (9), retries (28), model (24), time (8 or 1)
//	gen_metadata.data          one generation: chat model (1) with usage (4), retries (17),
//	                           model id (3), model name (19, 21), time (9 > 4); step indices (2)
//	trajectory_metadata_blob   the workspace (7), the parent conversation (5), the
//	                           subagent's name and task (8), the start time (2)
//
// No message text is read. One call is often written twice (in its step and in its
// generation, or in a parent and a child conversation); it counts once.

var antigravityDefaultRoots = []string{
	".gemini/antigravity", ".gemini/antigravity-cli", ".gemini/antigravity-ide",
	".gemini/antigravity-backup", ".gemini/antigravity-acp", ".config/antigravity",
}

// antigravityRoots are the folders that hold the conversation databases:
// $ANTIGRAVITY_DATA_DIR (comma-separated; a data folder or its conversations folder),
// else conversations/ of every default data folder.
func antigravityRoots(home string) []string {
	var dirs []string
	if env := strings.TrimSpace(os.Getenv("ANTIGRAVITY_DATA_DIR")); env != "" {
		for _, dir := range strings.Split(env, ",") {
			if dir = strings.TrimSpace(dir); dir == "" {
				continue
			}
			if isDir(filepath.Join(dir, "conversations")) {
				dir = filepath.Join(dir, "conversations")
			}
			dirs = append(dirs, dir)
		}
		return dirs
	}
	for _, root := range antigravityDefaultRoots {
		if dir := filepath.Join(home, filepath.FromSlash(root), "conversations"); isDir(dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// scanAntigravity reads each conversation database written since the last scan. A
// database is small, so a changed one is read whole again; calls already recorded are
// replaced, not added twice.
func (s *sessionScanner) scanAntigravity(since time.Time) {
	for _, dir := range s.antigravityRoots {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.db"))
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil || info.Size() == 0 {
				continue
			}
			stamp := info.ModTime()
			if wal, err := os.Stat(path + "-wal"); err == nil && wal.ModTime().After(stamp) {
				stamp = wal.ModTime()
			}
			seen, ok := s.antigravitySeen[path]
			if ok && !stamp.After(seen) || !ok && stamp.Before(since) {
				continue
			}
			if s.readAntigravityDB(path, stamp) == nil {
				s.antigravitySeen[path] = stamp
			}
		}
	}
}

// agUsage is one ModelUsage message.
type agUsage struct {
	ModelID                                   uint64
	Input, TotalOutput, CacheWrite, CacheRead int64
	Reasoning, VisibleOutput                  int64
	MessageID, ResponseID, ProviderMessageID  string
}

func (u agUsage) hasTokens() bool {
	return u.Input > 0 || u.TotalOutput > 0 || u.CacheWrite > 0 || u.CacheRead > 0 || u.Reasoning > 0 || u.VisibleOutput > 0
}

func (u agUsage) identities() []string {
	var ids []string
	for _, id := range []struct{ kind, value string }{{"response:", u.ResponseID}, {"provider:", u.ProviderMessageID}, {"message:", u.MessageID}} {
		if id.value != "" {
			ids = append(ids, id.kind+id.value)
		}
	}
	return ids
}

// mirrors tells a retry entry that only copies the primary usage: the retry list holds
// every attempt, the reported one included.
func (u agUsage) mirrors(primary agUsage) bool {
	if u.ModelID != primary.ModelID || u.Input != primary.Input || u.TotalOutput != primary.TotalOutput ||
		u.CacheWrite != primary.CacheWrite || u.CacheRead != primary.CacheRead || u.Reasoning != primary.Reasoning ||
		u.VisibleOutput != primary.VisibleOutput {
		return false
	}
	own := primary.identities()
	for _, id := range u.identities() {
		found := false
		for _, o := range own {
			found = found || o == id
		}
		if !found {
			return false
		}
	}
	return true
}

// agRecord is one row (a step or a generation) with its usages.
type agRecord struct {
	model    string
	usages   []agUsage // the primary one first, then the failed attempts
	when     time.Time
	fallback []string // identities when a usage has none of its own
}

func (s *sessionScanner) readAntigravityDB(path string, stamp time.Time) error {
	db, err := sql.Open("sqlite", sqliteReadOnlyURI(path))
	if err != nil {
		return err
	}
	defer db.Close()
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var workspace, parent, agentName, task string
	started := stamp
	var blob []byte
	if db.QueryRow("select data from trajectory_metadata_blob limit 1").Scan(&blob) == nil {
		f := protoFields(blob)
		workspace = fileURLPath(f.text(7))
		if workspace == "" {
			workspace = fileURLPath(protoFields(f.bytes(1)).text(1))
		}
		parent = f.text(5)
		sub := protoFields(f.bytes(8))
		agentName, task = sub.text(1), sub.text(2)
		if t, ok := protoTime(f.bytes(2)); ok {
			started = t
		}
	}

	var records []agRecord
	rows, err := db.Query("select idx, metadata from steps where metadata is not null order by idx")
	if err == nil {
		for rows.Next() {
			var idx int64
			if rows.Scan(&idx, &blob) != nil {
				continue
			}
			f := protoFields(blob)
			info := protoFields(f.bytes(24))
			model := info.text(12)
			if model == "" {
				model = info.text(8)
			}
			if model == "" && info.varint(1) != 0 {
				model = antigravityModelName(info.varint(1))
			}
			r := agRecord{model: model, fallback: []string{"step:" + itoa(idx)}}
			r.usages = agUsages(f.bytes(9), f.all(28))
			r.when, _ = protoTime(f.bytes(8))
			if r.when.IsZero() {
				r.when, _ = protoTime(f.bytes(1))
			}
			records = append(records, r)
		}
		rows.Close()
	}
	rows, err = db.Query("select idx, data from gen_metadata order by idx")
	if err != nil && len(records) == 0 {
		return err
	}
	if err == nil {
		current := ""
		for rows.Next() {
			var idx int64
			if rows.Scan(&idx, &blob) != nil {
				continue
			}
			root := protoFields(blob)
			chat := protoFields(root.bytes(1))
			r := agRecord{usages: agUsages(chat.bytes(4), chat.all(17))}
			model := chat.text(19)
			if model == "" {
				model = chat.text(21)
			}
			if model == "" && chat.varint(3) != 0 {
				model = antigravityModelName(chat.varint(3))
			}
			if model == "" && len(r.usages) > 0 && r.usages[0].ModelID != 0 {
				model = antigravityModelName(r.usages[0].ModelID)
			}
			if model != "" {
				current = model
			}
			r.model = current
			r.when, _ = protoTime(protoFields(chat.bytes(9)).bytes(4))
			for _, step := range root.varints(2) {
				r.fallback = append(r.fallback, "step:"+itoa(int64(step)))
			}
			if len(r.fallback) == 0 {
				r.fallback = []string{"gen:" + itoa(idx)}
			}
			records = append(records, r)
		}
		rows.Close()
	}

	project := s.projectOf(workspace)
	for _, r := range records {
		for i, u := range r.usages {
			if !u.hasTokens() {
				continue
			}
			ids := u.identities()
			if len(ids) == 0 && i == 0 {
				for _, f := range r.fallback {
					ids = append(ids, id+"/"+f)
				}
			}
			if len(ids) == 0 {
				continue // a failed attempt with nothing to match it by
			}
			model := r.model
			if u.ModelID != 0 {
				model = antigravityModelName(u.ModelID)
			}
			when := r.when
			if when.IsZero() {
				when = started
			}
			output := max(u.TotalOutput, u.VisibleOutput+u.Reasoning)
			call := sessionCall{
				When: when, Project: project, App: "antigravity", Model: normalizeAntigravityModel(model),
				Input: u.Input, Output: output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite,
				Session: id, Context: u.Input + u.CacheRead + u.CacheWrite,
			}
			if parent != "" && parent != id {
				call.Session, call.Sub, call.Agent = parent, true, shortID(id)
				if _, ok := s.agents[parent+"/"+call.Agent]; !ok {
					s.agents[parent+"/"+call.Agent] = agentMeta{Type: agentName, Description: task}
				}
			}
			s.recordAntigravity(ids, call, !r.when.IsZero())
		}
	}
	return nil
}

// recordAntigravity counts a call once however many copies of it there are: copies
// share an identity, and the largest numbers win.
func (s *sessionScanner) recordAntigravity(ids []string, call sessionCall, exactTime bool) {
	key := ""
	for _, id := range ids {
		if k, ok := s.antigravityKeys[id]; ok {
			key = k
			break
		}
	}
	if key == "" {
		key = "antigravity\x00" + ids[0]
	}
	for _, id := range ids {
		s.antigravityKeys[id] = key
	}
	if i, ok := s.index[key]; ok {
		old := s.calls[i]
		call.Input, call.Output = max(call.Input, old.Input), max(call.Output, old.Output)
		call.CacheRead, call.CacheWrite = max(call.CacheRead, old.CacheRead), max(call.CacheWrite, old.CacheWrite)
		call.Context = call.Input + call.CacheRead + call.CacheWrite
		// A time written with the call beats the conversation's start time; between
		// two of the same kind the earlier one wins.
		oldExact := s.antigravityExact[key]
		if oldExact && !exactTime || oldExact == exactTime && old.When.Before(call.When) {
			call.When = old.When
		}
		exactTime = exactTime || oldExact
		if call.Model == antigravityDefaultModel {
			call.Model = old.Model
		}
		if old.Sub && !call.Sub { // the parent's copy names the session it belongs to
			call.Session, call.Sub, call.Agent = old.Session, old.Sub, old.Agent
		}
	}
	s.antigravityExact[key] = exactTime
	s.record(key, call)
}

func agUsages(primary []byte, retries [][]byte) []agUsage {
	var list []agUsage
	var main agUsage
	hasMain := primary != nil
	if hasMain {
		main = parseAgUsage(primary)
		list = append(list, main)
	} else {
		list = append(list, agUsage{}) // keeps index 0 for the primary
	}
	dropped := !hasMain
	for _, r := range retries {
		inner := protoFields(r).bytes(2)
		if inner == nil {
			continue
		}
		u := parseAgUsage(inner)
		if !dropped && u.mirrors(main) {
			dropped = true
			continue
		}
		list = append(list, u)
	}
	return list
}

func parseAgUsage(blob []byte) agUsage {
	f := protoFields(blob)
	return agUsage{
		ModelID: f.varint(1), Input: int64(f.varint(2)), TotalOutput: int64(f.varint(3)),
		CacheWrite: int64(f.varint(4)), CacheRead: int64(f.varint(5)), Reasoning: int64(f.varint(9)),
		VisibleOutput: int64(f.varint(10)), MessageID: f.text(7), ResponseID: f.text(11), ProviderMessageID: f.text(12),
	}
}

// fileURLPath turns file:///D:/x into D:/x; other text is kept as it is.
func fileURLPath(s string) string {
	if !strings.HasPrefix(s, "file://") {
		return s
	}
	p := strings.TrimPrefix(s, "file://")
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// --- protobuf, just enough to read the fields above ---

type protoField struct {
	num    uint64
	varint uint64
	bytes  []byte
	isLen  bool
}

type protoMessage []protoField

var errProto = errors.New("bad protobuf")

func readVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i]&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, errProto
}

// protoFields decodes one message. A broken blob gives the fields read before the
// break, so one bad row never stops the rest.
func protoFields(b []byte) protoMessage {
	var m protoMessage
	for len(b) > 0 {
		tag, n, err := readVarint(b)
		if err != nil || tag>>3 == 0 {
			return m
		}
		b = b[n:]
		f := protoField{num: tag >> 3}
		switch tag & 7 {
		case 0:
			v, n, err := readVarint(b)
			if err != nil {
				return m
			}
			f.varint, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return m
			}
			b = b[8:]
		case 5:
			if len(b) < 4 {
				return m
			}
			b = b[4:]
		case 2:
			l, n, err := readVarint(b)
			if err != nil || l > uint64(len(b)-n) {
				return m
			}
			f.bytes, f.isLen, b = b[n:n+int(l)], true, b[n+int(l):]
		default:
			return m
		}
		m = append(m, f)
	}
	return m
}

// varint is the last value of a scalar field, as protobuf reads repeated scalars.
func (m protoMessage) varint(num uint64) uint64 {
	for i := len(m) - 1; i >= 0; i-- {
		if m[i].num == num && !m[i].isLen {
			return m[i].varint
		}
	}
	return 0
}

// varints is every value of a repeated number field, packed or not.
func (m protoMessage) varints(num uint64) []uint64 {
	var out []uint64
	for _, f := range m {
		if f.num != num {
			continue
		}
		if !f.isLen {
			out = append(out, f.varint)
			continue
		}
		for b := f.bytes; len(b) > 0; {
			v, n, err := readVarint(b)
			if err != nil {
				break
			}
			out, b = append(out, v), b[n:]
		}
	}
	return out
}

// bytes is the first message or string of a field; nil when there is none.
func (m protoMessage) bytes(num uint64) []byte {
	for _, f := range m {
		if f.num == num && f.isLen {
			if f.bytes == nil {
				return []byte{}
			}
			return f.bytes
		}
	}
	return nil
}

func (m protoMessage) all(num uint64) [][]byte {
	var out [][]byte
	for _, f := range m {
		if f.num == num && f.isLen {
			out = append(out, f.bytes)
		}
	}
	return out
}

// text is the last string of a field, when it is non-empty text.
func (m protoMessage) text(num uint64) string {
	for i := len(m) - 1; i >= 0; i-- {
		if m[i].num == num && m[i].isLen {
			if s := strings.TrimSpace(string(m[i].bytes)); s != "" && isText(m[i].bytes) {
				return string(m[i].bytes)
			}
			return ""
		}
	}
	return ""
}

func isText(b []byte) bool {
	for _, c := range string(b) {
		if c == '\uFFFD' || c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// protoTime reads a google.protobuf.Timestamp.
func protoTime(b []byte) (time.Time, bool) {
	if b == nil {
		return time.Time{}, false
	}
	f := protoFields(b)
	seconds := int64(f.varint(1))
	if seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, int64(min(f.varint(2), 999_999_999))), true
}

// --- model names ---

const antigravityDefaultModel = "gemini-internal-model"

// antigravityModelName names a numeric model id, as ccusage maps them.
func antigravityModelName(id uint64) string {
	switch id {
	case 246:
		return "gemini-2.5-pro"
	case 312:
		return "gemini-2.5-flash"
	case 313, 329:
		return "gemini-2.5-flash-thinking"
	case 330:
		return "gemini-2.5-flash-lite"
	case 281, 282:
		return "claude-4-sonnet"
	case 290, 291:
		return "claude-4-opus"
	case 333, 334:
		return "claude-4.5-sonnet"
	case 340, 341:
		return "claude-4.5-haiku"
	case 342:
		return "gpt-oss-120b-medium"
	case 1318:
		return "gemini-3.8-flash-high"
	case 1319:
		return "gemini-3.8-flash-medium"
	case 1320:
		return "gemini-3.8-flash-low"
	case 1298:
		return "gemini-3.7-flash-high"
	case 1299:
		return "gemini-3.7-flash-medium"
	case 1300:
		return "gemini-3.7-flash-low"
	case 1071:
		return "gemini-3.6-flash-high"
	case 1072:
		return "gemini-3.6-flash-medium"
	case 1073:
		return "gemini-3.6-flash-low"
	case 1417, 1418, 1419:
		return "gemini-4-argon"
	case 1402:
		return "claude-opus-5-5"
	case 1050:
		return "gemini-3.1-flash-lite"
	}
	if id >= 1000 {
		return "model_placeholder_m" + itoa(int64(id-1000))
	}
	return "antigravity-model-" + itoa(int64(id))
}

var antigravityAliases = map[string]string{
	"model_placeholder_m318": "gemini-3.8-flash-high", "model_placeholder_m319": "gemini-3.8-flash-medium",
	"model_placeholder_m320": "gemini-3.8-flash-low", "model_placeholder_m298": "gemini-3.7-flash-high",
	"model_placeholder_m299": "gemini-3.7-flash-medium", "model_placeholder_m300": "gemini-3.7-flash-low",
	"model_placeholder_m71": "gemini-3.6-flash-high", "model_placeholder_m72": "gemini-3.6-flash-medium",
	"model_placeholder_m73": "gemini-3.6-flash-low", "model_placeholder_m417": "gemini-4-argon",
	"model_placeholder_m418": "gemini-4-argon", "model_placeholder_m419": "gemini-4-argon",
	"model_placeholder_m402": "claude-opus-5-5", "model_placeholder_m50": "gemini-3.1-flash-lite",
	"model_placeholder_m26": "claude-opus-4-6", "model_placeholder_m35": "claude-sonnet-4-6",
	"model_placeholder_m36": "gemini-3.1-pro", "model_placeholder_m37": "gemini-3.1-pro",
	"model_placeholder_m16": "gemini-3.1-pro", "model_placeholder_m18": "gemini-3-flash-preview",
	"model_placeholder_m84": "gemini-3-flash-preview", "model_placeholder_m47": "gemini-3-flash-preview",
	"model_placeholder_m132": "gemini-3.5-flash-high", "model_placeholder_m133": "gemini-3.5-flash-high",
	"model_placeholder_m187": "gemini-3.5-flash-extra-low", "model_placeholder_m20": "gemini-3.5-flash-medium",
	"model_openai_gpt_oss_120b_medium": "gpt-oss-120b-medium",
	"gemini-pro-default":               "gemini-3.1-pro", "gemini-pro-agent": "gemini-3.1-pro",
	"gemini-3-flash-agent": "gemini-3.5-flash-high", "gemini-3-flash-agent-a": "gemini-3.5-flash-high",
	"gemini-3-flash-agent-b": "gemini-3.5-flash-high", "gemini-3-flash-a": "gemini-3.5-flash-high",
	"gemini-3-flash-b": "gemini-3.5-flash-high", "gemini-3-flash-c": "gemini-3-flash-preview",
	"gemini-3-flash": "gemini-3-flash-preview", "gemini-3.5-flash-low": "gemini-3.5-flash-medium",
	"gemini-3.1-pro-high": "gemini-3.1-pro", "gemini-3.1-pro-low": "gemini-3.1-pro",
	"gemini-3-pro-high": "gemini-3-pro", "gemini-3-pro-low": "gemini-3-pro",
	"gemini-3-flash-thinking": "gemini-3.6-flash", "gemini-3.6-flash-thinking": "gemini-3.6-flash",
}

var effortLabel = regexp.MustCompile(`^(gemini \d+(?:\.\d+)? flash) \((high|medium|low)\)$`)

// normalizeAntigravityModel turns a display name ("Gemini 3.8 Flash (High)") or an
// internal one into the id the price list uses.
func normalizeAntigravityModel(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return antigravityDefaultModel
	}
	if m := effortLabel.FindStringSubmatch(name); m != nil {
		name = m[1] + " " + m[2]
	} else if i := strings.Index(name, "("); i > 0 {
		name = strings.TrimSpace(name[:i])
	}
	name = strings.TrimSuffix(name, " thinking")
	name = strings.ReplaceAll(name, " ", "-")
	if alias, ok := antigravityAliases[name]; ok {
		return alias
	}
	if strings.HasPrefix(name, "gemini-") || strings.HasPrefix(name, "claude-") || strings.HasPrefix(name, "gpt-") {
		return name
	}
	return strings.TrimSpace(raw)
}
