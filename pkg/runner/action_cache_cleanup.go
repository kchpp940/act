package runner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CleanupConfig struct {
	MaxAge         time.Duration
	MinAccessCount int
	RepoSource     string
	MaxSize        int64
	Type           string
	Ref            string
	WorkflowPath   string
}

type CleanupResult struct {
	RemovedEntries []*ActionCacheEntry
	FreedBytes     int64
}

func matchesFilters(e *ActionCacheEntry, cfg CleanupConfig) bool {
	if cfg.Type != "" && e.Type != cfg.Type {
		return false
	}
	if cfg.Ref != "" && e.Ref != cfg.Ref {
		return false
	}
	if cfg.WorkflowPath != "" && !strings.HasPrefix(e.WorkflowPath, cfg.WorkflowPath) {
		return false
	}
	if cfg.RepoSource != "" && e.RepoSource != cfg.RepoSource {
		return false
	}
	return true
}

func CleanupCache(meta *ActionCacheMetaManager, cfg CleanupConfig) (*CleanupResult, error) {
	meta.SyncFromDisk()

	allEntries := meta.GetEntries()
	entries := make([]*ActionCacheEntry, 0, len(allEntries))
	for _, e := range allEntries {
		if matchesFilters(e, cfg) {
			entries = append(entries, e)
		}
	}

	entryMap := make(map[string]*ActionCacheEntry, len(entries))
	for _, e := range entries {
		entryMap[e.ExecutionKey] = e
	}

	removedSet := make(map[string]bool)

	if cfg.MaxAge > 0 {
		cutoff := time.Now().Add(-cfg.MaxAge)
		for _, e := range entries {
			if e.LastAccessAt.Before(cutoff) && !removedSet[e.ExecutionKey] {
				removedSet[e.ExecutionKey] = true
			}
		}
	}

	if cfg.MinAccessCount > 0 {
		for _, e := range entries {
			if e.AccessCount < cfg.MinAccessCount && !removedSet[e.ExecutionKey] {
				removedSet[e.ExecutionKey] = true
			}
		}
	}

	if cfg.MaxSize > 0 {
		var totalSize int64
		for _, e := range entries {
			if !removedSet[e.ExecutionKey] {
				totalSize += e.Size
			}
		}
		if totalSize > cfg.MaxSize {
			sorted := make([]*ActionCacheEntry, 0, len(entries))
			for _, e := range entries {
				if !removedSet[e.ExecutionKey] {
					sorted = append(sorted, e)
				}
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].LastAccessAt.Before(sorted[j].LastAccessAt)
			})
			for _, e := range sorted {
				if totalSize <= cfg.MaxSize {
					break
				}
				totalSize -= e.Size
				removedSet[e.ExecutionKey] = true
			}
		}
	}

	hasFilterCriteria := cfg.Type != "" || cfg.Ref != "" || cfg.WorkflowPath != "" || cfg.RepoSource != ""
	hasPruneCriteria := cfg.MaxAge > 0 || cfg.MinAccessCount > 0 || cfg.MaxSize > 0
	if hasFilterCriteria && !hasPruneCriteria {
		for _, e := range entries {
			if !removedSet[e.ExecutionKey] {
				removedSet[e.ExecutionKey] = true
			}
		}
	}

	result := &CleanupResult{}
	for execKey := range removedSet {
		entry, exists := entryMap[execKey]
		if !exists {
			continue
		}
		gitPath := filepath.Join(meta.CachePath(), entry.ExecutionKey+".git")
		size := dirSize(gitPath)
		if err := os.RemoveAll(gitPath); err != nil {
			continue
		}
		result.RemovedEntries = append(result.RemovedEntries, entry)
		result.FreedBytes += size
		meta.RemoveEntry(execKey)
	}

	sort.Slice(result.RemovedEntries, func(i, j int) bool {
		return result.RemovedEntries[i].ExecutionKey < result.RemovedEntries[j].ExecutionKey
	})
	return result, nil
}

func TotalCacheSize(meta *ActionCacheMetaManager) int64 {
	meta.SyncFromDisk()
	entries := meta.GetEntries()
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	return total
}
