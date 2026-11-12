package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// What filled the context for nothing. A tool result stays in the conversation
// and is sent again with every later turn, so one large result costs far more
// than its own size.

const (
	bigResultTokens = 8000 // a result this large is worth a line (~32 KB of text)
	rereadTimes     = 3    // the same whole file read this often in one session
	wasteListed     = 8
)

// digestResult is one large tool result.
type digestResult struct {
	session string
	tool    string
	input   string
	tokens  int64 // estimate: characters / 4
}

// countRead notes a Read of a whole file (no offset, no limit). Reading the
// same file whole again and again is the commonest way to waste context.
func (r *digestReport) countRead(session string, block digestBlock, root string) {
	if block.Name != "Read" {
		return
	}
	var input map[string]any
	if json.Unmarshal(block.Input, &input) != nil || input["offset"] != nil || input["limit"] != nil || input["pages"] != nil {
		return
	}
	if file := toolInput("Read", block.Input, root); file != "" {
		r.reads[session+"\x00"+strings.ReplaceAll(file, `\`, "/")]++
	}
}

func (r *digestReport) printWaste(out io.Writer) {
	type group struct {
		tool, input string
		count       int
		tokens      int64
		sessions    []string
	}
	groups := map[string]*group{}
	var total int64
	for _, result := range r.big {
		key := result.tool + "\x00" + numberPattern.ReplaceAllString(result.input, "N")
		g := groups[key]
		if g == nil {
			g = &group{tool: result.tool, input: result.input}
			groups[key] = g
		}
		g.count++
		g.tokens += result.tokens
		g.sessions = appendUnique(g.sessions, shortSession(result.session))
		total += result.tokens
	}
	type reread struct {
		session, file string
		count         int
	}
	var rereads []reread
	for key, count := range r.reads {
		if count >= rereadTimes {
			session, file, _ := strings.Cut(key, "\x00")
			rereads = append(rereads, reread{session, file, count})
		}
	}
	if len(groups) == 0 && len(rereads) == 0 {
		return
	}
	fmt.Fprintln(out, "WASTE (context spent for little; sizes are estimates, characters / 4):")
	if len(groups) > 0 {
		list := make([]*group, 0, len(groups))
		for _, g := range groups {
			list = append(list, g)
		}
		sort.Slice(list, func(a, b int) bool {
			if list[a].tokens != list[b].tokens {
				return list[a].tokens > list[b].tokens
			}
			return list[a].input < list[b].input
		})
		fmt.Fprintf(out, "  BIG RESULTS (one tool result of %s+ tokens): %d results, ~%s tokens\n",
			compactTokens(bigResultTokens), len(r.big), compactTokens(total))
		for i, g := range list {
			if i == wasteListed {
				fmt.Fprintf(out, "    ... and %d more\n", len(list)-wasteListed)
				break
			}
			sessions := g.sessions
			more := ""
			if len(sessions) > 3 {
				more = fmt.Sprintf(" +%d", len(sessions)-3)
				sessions = sessions[:3]
			}
			fmt.Fprintf(out, "    ~%-6s %dx  %s `%s`  sessions: %s%s\n", compactTokens(g.tokens), g.count, g.tool, g.input,
				strings.Join(sessions, ", "), more)
		}
	}
	if len(rereads) > 0 {
		sort.Slice(rereads, func(a, b int) bool {
			if rereads[a].count != rereads[b].count {
				return rereads[a].count > rereads[b].count
			}
			return rereads[a].session+rereads[a].file < rereads[b].session+rereads[b].file
		})
		fmt.Fprintf(out, "  READ AGAIN (the same whole file read %d+ times in one session):\n", rereadTimes)
		for i, item := range rereads {
			if i == wasteListed {
				fmt.Fprintf(out, "    ... and %d more\n", len(rereads)-wasteListed)
				break
			}
			fmt.Fprintf(out, "    %dx  %s  %s\n", item.count, shortSession(item.session), item.file)
		}
	}
}
