package main

import (
	"bytes"
	"os"
	"testing"
)

func TestOrchestratorSkill(t *testing.T) {
	const repoPath = ".claude/skills/orca-orchestrator/SKILL.md"
	const kitPath = "kit/orca/dot-claude/skills/orca-orchestrator/SKILL.md"

	repo, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	kit, err := os.ReadFile(kitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repo, kit) {
		t.Fatalf("%s and %s must match byte for byte", repoPath, kitPath)
	}
	for _, command := range []string{"codex exec", "agy -p"} {
		if !bytes.Contains(repo, []byte(command)) {
			t.Errorf("%s and %s must name the %q worker command", repoPath, kitPath, command)
		}
	}
}
