package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nektos/act/pkg/runner"
	"github.com/spf13/cobra"
)

type CacheEntry struct {
	*runner.ActionCacheEntry
	IsOrphan bool `json:"is_orphan"`
}

func newCacheCommand() *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the action cache",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List cached actions and workflows with metadata",
		RunE:  runCacheList,
	}
	listCmd.Flags().String("format", "table", "Output format: table or json")
	listCmd.Flags().Bool("all", false, "Include orphan entries (directories on disk without metadata)")

	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune the action cache based on cleanup strategies",
		RunE:  runCachePrune,
	}
	pruneCmd.Flags().String("type", "", "Filter by entry type: action or workflow")
	pruneCmd.Flags().String("ref", "", "Filter by reference (e.g., v3, main, refs/tags/v1.0)")
	pruneCmd.Flags().String("workflow-path", "", "Filter by workflow path prefix (e.g., .github/workflows/deploy)")
	pruneCmd.Flags().String("max-age", "", "Remove entries not accessed within this duration (e.g., 30d, 720h)")
	pruneCmd.Flags().Int("min-access-count", 0, "Remove entries with access count below this threshold")
	pruneCmd.Flags().String("repo-source", "", "Remove entries from the specified repository source (e.g., github.com)")
	pruneCmd.Flags().String("max-size", "", "Remove least-recently-used entries until total cache is below this size (e.g., 1GB, 500MB)")
	pruneCmd.Flags().Bool("dry-run", false, "Show what would be removed without actually removing")
	pruneCmd.Flags().Bool("prune-orphans", false, "Also remove orphan directories on disk that have no metadata records")

	cacheCmd.AddCommand(listCmd)
	cacheCmd.AddCommand(pruneCmd)

	return cacheCmd
}

func defaultActionCachePath() string {
	return filepath.Join(CacheHomeDir, "act")
}

func getCacheMeta(cmd *cobra.Command) *runner.ActionCacheMetaManager {
	cachePath, _ := cmd.Flags().GetString("action-cache-path")
	return runner.NewActionCacheMetaManager(cachePath)
}

func getCachePath(cmd *cobra.Command) string {
	cachePath, _ := cmd.Flags().GetString("action-cache-path")
	return cachePath
}

func scanOrphanEntries(cachePath string, meta *runner.ActionCacheMetaManager) []string {
	metaKeys := make(map[string]bool)
	for _, e := range meta.GetEntries() {
		metaKeys[e.ExecutionKey] = true
	}

	var orphans []string
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return orphans
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == ".cache-meta.json" || name == ".cache-meta.json.tmp" {
			continue
		}
		if !entry.IsDir() || !stringsHasSuffix(name, ".git") {
			continue
		}
		key := stringsTrimSuffix(name, ".git")
		if !metaKeys[key] {
			orphans = append(orphans, key)
		}
	}
	return orphans
}

func runCacheList(cmd *cobra.Command, _ []string) error {
	meta := getCacheMeta(cmd)
	cachePath := getCachePath(cmd)
	showAll, _ := cmd.Flags().GetBool("all")

	meta.SyncFromDisk()
	entries := meta.GetEntries()

	allEntries := make([]CacheEntry, 0, len(entries))
	for _, e := range entries {
		allEntries = append(allEntries, CacheEntry{ActionCacheEntry: e, IsOrphan: false})
	}

	var orphans []string
	if showAll {
		orphans = scanOrphanEntries(cachePath, meta)
		for _, key := range orphans {
			gitPath := filepath.Join(cachePath, key+".git")
			size := dirSize(gitPath)
			modTime := time.Now()
			if fi, err := os.Stat(gitPath); err == nil {
				modTime = fi.ModTime()
			}
			allEntries = append(allEntries, CacheEntry{
				ActionCacheEntry: &runner.ActionCacheEntry{
					Type:           "unknown",
					NormalizedUses: key + " (orphan)",
					RepoKey:        "unknown",
					ExecutionKey:   key,
					Size:           size,
					LastAccessAt:   modTime,
					AccessCount:    0,
				},
				IsOrphan: true,
			})
		}
	}

	format, _ := cmd.Flags().GetString("format")

	if format == "json" {
		data, err := json.MarshalIndent(allEntries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tUSES\tREPO KEY\tREF\tWORKFLOW\tSOURCE\tEXEC KEY\tSIZE\tACCESSES\tLAST ACCESS\tSTATUS")
	for _, e := range allEntries {
		status := "managed"
		if e.IsOrphan {
			status = "orphan"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			e.Type,
			e.NormalizedUses,
			e.RepoKey,
			e.Ref,
			e.WorkflowPath,
			e.RepoSource,
			e.ExecutionKey,
			formatSize(e.Size),
			e.AccessCount,
			e.LastAccessAt.Format(time.DateOnly),
			status,
		)
	}
	w.Flush()

	total := runner.TotalCacheSize(meta)
	fmt.Printf("\nTotal: %s in %d managed entries", formatSize(total), len(entries))
	if showAll && len(orphans) > 0 {
		fmt.Printf(", %d orphan entries", len(orphans))
	}
	fmt.Println()
	return nil
}

func runCachePrune(cmd *cobra.Command, _ []string) error {
	meta := getCacheMeta(cmd)
	cachePath := getCachePath(cmd)

	cfg := runner.CleanupConfig{}

	cfg.Type, _ = cmd.Flags().GetString("type")
	if cfg.Type != "" && cfg.Type != "action" && cfg.Type != "workflow" {
		return fmt.Errorf("invalid --type value %q: must be 'action' or 'workflow'", cfg.Type)
	}
	cfg.Ref, _ = cmd.Flags().GetString("ref")
	cfg.WorkflowPath, _ = cmd.Flags().GetString("workflow-path")

	maxAgeStr, _ := cmd.Flags().GetString("max-age")
	if maxAgeStr != "" {
		d, err := parseDuration(maxAgeStr)
		if err != nil {
			return fmt.Errorf("invalid --max-age value %q: %w", maxAgeStr, err)
		}
		cfg.MaxAge = d
	}

	cfg.MinAccessCount, _ = cmd.Flags().GetInt("min-access-count")

	cfg.RepoSource, _ = cmd.Flags().GetString("repo-source")

	maxSizeStr, _ := cmd.Flags().GetString("max-size")
	if maxSizeStr != "" {
		s, err := parseSize(maxSizeStr)
		if err != nil {
			return fmt.Errorf("invalid --max-size value %q: %w", maxSizeStr, err)
		}
		cfg.MaxSize = s
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	pruneOrphans, _ := cmd.Flags().GetBool("prune-orphans")

	if dryRun {
		return runCachePruneDryRun(meta, cfg, cachePath, pruneOrphans)
	}

	result, err := runner.CleanupCache(meta, cfg)
	if err != nil {
		return err
	}

	orphanCount := 0
	orphanFreed := int64(0)
	var orphanedKeys []string
	if pruneOrphans {
		orphans := scanOrphanEntries(cachePath, meta)
		for _, key := range orphans {
			gitPath := filepath.Join(cachePath, key+".git")
			size := dirSize(gitPath)
			if err := os.RemoveAll(gitPath); err == nil {
				orphanCount++
				orphanFreed += size
				orphanedKeys = append(orphanedKeys, key)
			}
		}
	}

	totalRemoved := len(result.RemovedEntries) + orphanCount
	totalFreed := result.FreedBytes + orphanFreed

	if totalRemoved == 0 {
		fmt.Println("No entries to prune.")
		return nil
	}

	fmt.Printf("Removed %d entries, freed %s\n", totalRemoved, formatSize(totalFreed))
	for _, e := range result.RemovedEntries {
		fmt.Printf("  - %s [%s] → %s.git\n", e.NormalizedUses, e.Type, e.ExecutionKey)
	}
	for _, k := range orphanedKeys {
		fmt.Printf("  - %s [orphan] → %s.git\n", k, k)
	}
	return nil
}

func runCachePruneDryRun(meta *runner.ActionCacheMetaManager, cfg runner.CleanupConfig, cachePath string, pruneOrphans bool) error {
	meta.SyncFromDisk()
	allEntries := meta.GetEntries()

	entries := make([]*runner.ActionCacheEntry, 0, len(allEntries))
	for _, e := range allEntries {
		if cfg.Type != "" && e.Type != cfg.Type {
			continue
		}
		if cfg.Ref != "" && e.Ref != cfg.Ref {
			continue
		}
		if cfg.WorkflowPath != "" && !strings.HasPrefix(e.WorkflowPath, cfg.WorkflowPath) {
			continue
		}
		if cfg.RepoSource != "" && e.RepoSource != cfg.RepoSource {
			continue
		}
		entries = append(entries, e)
	}

	var candidateEntries []*runner.ActionCacheEntry
	candidateSet := make(map[string]bool)

	if cfg.MaxAge > 0 {
		cutoff := time.Now().Add(-cfg.MaxAge)
		for _, e := range entries {
			if e.LastAccessAt.Before(cutoff) && !candidateSet[e.ExecutionKey] {
				candidateEntries = append(candidateEntries, e)
				candidateSet[e.ExecutionKey] = true
			}
		}
	}

	if cfg.MinAccessCount > 0 {
		for _, e := range entries {
			if e.AccessCount < cfg.MinAccessCount && !candidateSet[e.ExecutionKey] {
				candidateEntries = append(candidateEntries, e)
				candidateSet[e.ExecutionKey] = true
			}
		}
	}

	if cfg.MaxSize > 0 {
		var totalSize int64
		for _, e := range entries {
			if !candidateSet[e.ExecutionKey] {
				totalSize += e.Size
			}
		}
		if totalSize > cfg.MaxSize {
			sorted := make([]*runner.ActionCacheEntry, 0, len(entries))
			for _, e := range entries {
				if !candidateSet[e.ExecutionKey] {
					sorted = append(sorted, e)
				}
			}
			for _, e := range sorted {
				if totalSize <= cfg.MaxSize {
					break
				}
				totalSize -= e.Size
				candidateEntries = append(candidateEntries, e)
				candidateSet[e.ExecutionKey] = true
			}
		}
	}

	hasOtherCriteria := cfg.MaxAge > 0 || cfg.MinAccessCount > 0 || cfg.MaxSize > 0
	if !hasOtherCriteria && len(entries) > 0 {
		for _, e := range entries {
			if !candidateSet[e.ExecutionKey] {
				candidateEntries = append(candidateEntries, e)
				candidateSet[e.ExecutionKey] = true
			}
		}
	}

	var orphans []string
	if pruneOrphans {
		orphans = scanOrphanEntries(cachePath, meta)
	}

	totalCount := len(candidateEntries) + len(orphans)
	if totalCount == 0 {
		fmt.Println("No entries would be pruned.")
		return nil
	}

	fmt.Printf("Would remove %d entries:\n", totalCount)
	for _, e := range candidateEntries {
		fmt.Printf("  - %s [%s] → %s.git\n", e.NormalizedUses, e.Type, e.ExecutionKey)
	}
	for _, k := range orphans {
		fmt.Printf("  - %s [orphan] → %s.git\n", k, k)
	}
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count: %s", daysStr)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	multipliers := map[string]int64{
		"GB":  1 << 30,
		"MB":  1 << 20,
		"KB":  1 << 10,
		"GIB": 1 << 30,
		"MIB": 1 << 20,
		"KIB": 1 << 10,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, suffix))
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size number: %s", numStr)
			}
			return int64(num * float64(mult)), nil
		}
	}

	num, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %s", s)
	}
	return num, nil
}

func formatSize(b int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func stringsTrimSuffix(s, suffix string) string {
	if stringsHasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}
