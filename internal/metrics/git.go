package metrics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Git struct {
	Branch                                                string
	Ahead, Behind, Staged, Modified, Untracked, Conflicts int
	Files                                                 []string
	Error                                                 string
}

func (g Git) Summary() string {
	if g.Error != "" {
		return g.Error
	}
	parts := []string{}
	for _, item := range []struct {
		n    int
		name string
	}{{g.Staged, "staged"}, {g.Modified, "modified"}, {g.Untracked, "new"}, {g.Conflicts, "conflicts"}} {
		if item.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", item.n, item.name))
		}
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}
func ReadGit(ctx context.Context, cwd string) Git {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "-C", cwd, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=normal")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := cmd.Output()
	if err != nil {
		return Git{Error: "Git unavailable / not a repository"}
	}
	return ParseGit(output)
}
func ParseGit(output []byte) Git {
	result := Git{Branch: "—"}
	records := strings.Split(string(output), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		switch {
		case strings.HasPrefix(record, "# branch.head "):
			result.Branch = strings.TrimPrefix(record, "# branch.head ")
		case strings.HasPrefix(record, "# branch.ab "):
			fields := strings.Fields(record)
			if len(fields) == 4 {
				result.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
				result.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			}
		case strings.HasPrefix(record, "? "):
			result.Untracked++
			result.Files = append(result.Files, record)
		case strings.HasPrefix(record, "1 "), strings.HasPrefix(record, "2 "), strings.HasPrefix(record, "u "):
			count := 9
			if record[0] == '2' {
				count = 10
			}
			if record[0] == 'u' {
				count = 11
			}
			fields := strings.SplitN(record, " ", count)
			if len(fields) != count || len(fields[1]) != 2 {
				continue
			}
			xy, path := fields[1], fields[count-1]
			if record[0] == 'u' {
				result.Conflicts++
			} else {
				if xy[0] != '.' {
					result.Staged++
				}
				if xy[1] != '.' {
					result.Modified++
				}
			}
			if record[0] == '2' && i+1 < len(records) {
				i++
				path = records[i] + " → " + path
			}
			result.Files = append(result.Files, xy+" "+path)
		}
	}
	return result
}
