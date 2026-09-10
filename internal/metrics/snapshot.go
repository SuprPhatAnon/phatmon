package metrics

import (
	"context"
	"sync"

	"phatmon/internal/codex"
)

// Snapshot bounds concurrent Git commands and avoids duplicate directory reads.
func Snapshot(ctx context.Context, threads []codex.Thread, home string, cache *UsageCache) (map[string]*codex.Usage, map[string]Git) {
	usage, git := map[string]*codex.Usage{}, map[string]Git{}
	var mu sync.Mutex
	var group sync.WaitGroup
	jobs := make(chan string)
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			for path := range jobs {
				value := ReadGit(ctx, path)
				mu.Lock()
				git[path] = value
				mu.Unlock()
			}
		}()
	}
	paths := map[string]bool{}
	for _, thread := range threads {
		if ctx.Err() != nil {
			break
		}
		if value := cache.Read(thread.Path, home); value != nil {
			usage[thread.ID] = value
		}
		if thread.Cwd != "" && !paths[thread.Cwd] {
			paths[thread.Cwd] = true
			select {
			case jobs <- thread.Cwd:
			case <-ctx.Done():
			}
		}
	}
	close(jobs)
	group.Wait()
	return usage, git
}
