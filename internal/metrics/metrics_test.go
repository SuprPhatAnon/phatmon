package metrics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"phatmon/internal/codex"
)

func TestGitPorcelainRenamesConflictsAndSpaces(t *testing.T) {
	data := strings.Join([]string{
		"# branch.head feature/test", "# branch.ab +3 -2",
		"1 MM N... 100644 100644 100644 abc def file with spaces.go",
		"2 R. N... 100644 100644 100644 abc def R100 new name.go", "old name.go",
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.go",
		"? new\nfile.go", "",
	}, "\x00")
	got := ParseGit([]byte(data))
	if got.Branch != "feature/test" || got.Ahead != 3 || got.Behind != 2 || got.Staged != 2 || got.Modified != 1 || got.Conflicts != 1 || got.Untracked != 1 {
		t.Fatalf("unexpected status: %#v", got)
	}
	if got.Files[1] != "R. old name.go → new name.go" {
		t.Fatal(got.Files)
	}
}
func TestActualGitWorkingTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing")
	}
	dir := t.TempDir()
	if output, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file with spaces"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", dir, "add", "file with spaces").CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	os.WriteFile(filepath.Join(dir, "file with spaces"), []byte("changed"), 0600)
	got := ReadGit(context.Background(), dir)
	if got.Staged != 1 || got.Modified != 1 || got.Error != "" {
		t.Fatalf("%#v", got)
	}
}
func TestRolloutUsageAndContainment(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout.jsonl")
	token := `{"type":"event_msg","timestamp":"2026-09-10T16:00:00Z","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":50000,"input_tokens":42000,"output_tokens":8000},"last_token_usage":{"total_tokens":10000},"model_context_window":100000}}}`
	os.WriteFile(path, []byte("invalid\n"+token+"\n{partial"), 0600)
	usage := ReadUsage(path, root)
	if usage == nil {
		t.Fatal("no usage")
	}
	percent, ok := usage.ContextPercent()
	if !ok || percent != 10 || usage.Total.TotalTokens != 50000 {
		t.Fatalf("wrong context metric: %#v %v", usage, percent)
	}
	if ReadUsage(path, t.TempDir()) != nil {
		t.Fatal("read rollout outside home")
	}
	link := filepath.Join(t.TempDir(), "link.jsonl")
	os.Symlink(path, link)
	if ReadUsage(link, filepath.Dir(link)) != nil {
		t.Fatal("followed symlink outside home")
	}
	cache := NewUsageCache()
	if cache.Read(path, root) == nil {
		t.Fatal("cache read")
	}
	os.WriteFile(path, []byte("broken"), 0600)
	if cache.Read(path, root) != nil {
		t.Fatal("cache ignored changed file")
	}
}
func TestQuotaLabelsUseDurations(t *testing.T) {
	for minutes, want := range map[int]string{10080: "Weekly", 300: "5h", 15: "0.25h"} {
		w := codex.Window{WindowDurationMins: &minutes}
		if w.Label() != want {
			t.Fatal(fmt.Sprintf("%d: %s", minutes, w.Label()))
		}
	}
	if _, ok := (*codex.Usage)(nil).ContextPercent(); ok {
		t.Fatal("unknown context became zero")
	}
}
