package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedToolkitAllPlatforms(t *testing.T) {
	for _, platform := range []string{"windows", "linux", "darwin"} {
		for _, action := range []string{"install", "uninstall"} {
			for _, module := range []string{"cli", "wm"} {
				path, err := toolkitScript(platform, action, module)
				if err != nil {
					t.Fatal(err)
				}
				data, err := bundledToolkit.ReadFile(path)
				if err != nil || len(data) < 100 {
					t.Fatalf("missing %s: %v", path, err)
				}
				if platform == "windows" && bytes.Contains(data, []byte("%~f0")) {
					t.Fatal("batch wrapper was not removed")
				}
			}
		}
	}
	for _, args := range [][3]string{{"windows", "delete", "wm"}, {"windows", "install", "../secret"}, {"freebsd", "install", "cli"}} {
		if _, err := toolkitScript(args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid action admitted")
		}
	}
}

func TestToolkitDryRunDoesNotExtract(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	var out bytes.Buffer
	if err := runToolkit([]string{"install", "cli", "-groups", "1,4", "-dry-run"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1, 4") || !strings.Contains(out.String(), "Dry run") {
		t.Fatal("missing preview")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Fatal("dry run extracted scripts")
	}
	if err := runToolkit([]string{"uninstall", "wm"}, strings.NewReader("n\n"), &out); err != nil {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatal("cancelled action extracted scripts")
	}
	for _, args := range [][]string{{"install", "cli", "-groups", "5"}, {"install", "wm", "-groups", "1"}, {"install", "wm", "extra"}} {
		if err := runToolkit(args, strings.NewReader(""), &out); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}

func TestMenuDoesNotReadChildInputAhead(t *testing.T) {
	in := strings.NewReader("2\nremaining-input\n")
	var out bytes.Buffer
	if answer := menuAnswer(in, &out, "Choose"); answer != "2" {
		t.Fatal(answer)
	}
	if answer := menuAnswer(in, &out, "Child"); answer != "remaining-input" {
		t.Fatal("menu consumed child input")
	}
	if err := runApp(nil, strings.NewReader("0\n"), &out); err != nil {
		t.Fatal(err)
	}
}

func TestToolkitChildExitStatusGuards(t *testing.T) {
	for _, path := range []string{"toolkit/windows/install-cli.ps1", "toolkit/windows/install-wm.ps1", "toolkit/unix/install-cli.sh"} {
		data, _ := bundledToolkit.ReadFile(filepath.ToSlash(path))
		if !bytes.Contains(data, []byte("exit 1")) {
			t.Fatalf("%s cannot signal installation failure", path)
		}
	}
}

func TestToolkitWSLModule(t *testing.T) {
	path, err := toolkitScript("windows", "install", "wsl")
	if err != nil || path != "toolkit/windows/install-wsl.ps1" {
		t.Fatalf("got %q, %v", path, err)
	}
	data, err := bundledToolkit.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# >>> wisjoy-wsl >>>", "--no-launch", "exit 1"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("install-wsl.ps1 lacks %q", want)
		}
	}
	for _, args := range [][3]string{{"windows", "uninstall", "wsl"}, {"linux", "install", "wsl"}, {"darwin", "install", "wsl"}} {
		if _, err := toolkitScript(args[0], args[1], args[2]); err == nil {
			t.Fatalf("%v admitted", args)
		}
	}
	var out bytes.Buffer
	if err := runToolkit([]string{"install", "wsl", "-groups", "1"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("-groups admitted for wsl")
	}
}
