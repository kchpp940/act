package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	CacheEntryTypeAction   = "action"
	CacheEntryTypeWorkflow = "workflow"
)

type ActionCacheEntry struct {
	Type           string    `json:"type"`
	NormalizedUses string    `json:"normalized_uses"`
	RepoKey        string    `json:"repo_key"`
	Ref            string    `json:"ref"`
	SHA            string    `json:"sha"`
	RepoURL        string    `json:"repo_url"`
	RepoSource     string    `json:"repo_source"`
	ExecutionKey   string    `json:"execution_key"`
	WorkflowPath   string    `json:"workflow_path,omitempty"`
	LastAccessAt   time.Time `json:"last_access_at"`
	AccessCount    int       `json:"access_count"`
	Size           int64     `json:"size"`
	CreatedAt      time.Time `json:"created_at"`
}

type ActionCacheMetaManager struct {
	mu       sync.Mutex
	path     string
	metaFile string
	entries  map[string]*ActionCacheEntry
}

func NewActionCacheMetaManager(cachePath string) *ActionCacheMetaManager {
	m := &ActionCacheMetaManager{
		path:     cachePath,
		metaFile: filepath.Join(cachePath, ".cache-meta.json"),
		entries:  make(map[string]*ActionCacheEntry),
	}
	m.load()
	return m
}

func (m *ActionCacheMetaManager) load() {
	data, err := os.ReadFile(m.metaFile)
	if err != nil {
		return
	}
	var entries map[string]*ActionCacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	m.entries = entries
	for _, e := range m.entries {
		if e.ExecutionKey == "" {
			e.ExecutionKey = SafeFilename(e.RepoKey)
		}
		if e.Type == "" {
			e.Type = inferEntryType(e.RepoKey)
		}
		if e.NormalizedUses == "" && e.RepoKey != "" {
			e.NormalizedUses = buildNormalizedUses(e.Type, e.RepoKey, e.Ref, e.WorkflowPath)
		}
	}
}

func inferEntryType(repoKey string) string {
	if strings.Contains(repoKey, "@") {
		return CacheEntryTypeWorkflow
	}
	return CacheEntryTypeAction
}

func buildNormalizedUses(entryType, repoKey, ref, workflowPath string) string {
	if entryType == CacheEntryTypeWorkflow && workflowPath != "" {
		return repoKey + "/" + workflowPath + "@" + ref
	}
	if strings.Contains(repoKey, "@") {
		return repoKey
	}
	if ref != "" {
		return repoKey + "@" + ref
	}
	return repoKey
}

func (m *ActionCacheMetaManager) save() error {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.metaFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.metaFile)
}

func parseCacheDir(cacheDir string) (entryType, repoKey, ref string) {
	atIdx := strings.LastIndex(cacheDir, "@")
	if atIdx < 0 {
		return CacheEntryTypeAction, cacheDir, ""
	}
	prefix := cacheDir[:atIdx]
	suffix := cacheDir[atIdx+1:]
	if strings.Contains(prefix, "/") {
		return CacheEntryTypeWorkflow, prefix, suffix
	}
	return CacheEntryTypeAction, cacheDir, ""
}

func (m *ActionCacheMetaManager) RecordAccess(cacheDir, rawURL, ref, sha string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	detectedType, repoKey, _ := parseCacheDir(cacheDir)
	entryType := detectedType
	executionKey := SafeFilename(cacheDir)
	key := executionKey

	now := time.Now()

	repoSource := ""
	repoURL := rawURL
	if rawURL != "" {
		if u, err := url.Parse(rawURL); err == nil {
			repoSource = u.Host
		}
	}

	entry, exists := m.entries[key]
	if !exists {
		normalizedUses := buildNormalizedUses(entryType, repoKey, ref, "")
		entry = &ActionCacheEntry{
			Type:           entryType,
			NormalizedUses: normalizedUses,
			RepoKey:        repoKey,
			Ref:            ref,
			SHA:            sha,
			RepoURL:        repoURL,
			RepoSource:     repoSource,
			ExecutionKey:   executionKey,
			CreatedAt:      now,
		}
		m.entries[key] = entry
	} else {
		if sha != "" {
			entry.SHA = sha
		}
		if repoURL != "" {
			entry.RepoURL = repoURL
		}
		if repoSource != "" {
			entry.RepoSource = repoSource
		}
		if ref != "" {
			entry.Ref = ref
		}
	}

	entry.LastAccessAt = now
	entry.AccessCount++

	gitPath := filepath.Join(m.path, executionKey+".git")
	if _, err := os.Stat(gitPath); err == nil {
		entry.Size = dirSize(gitPath)
	}

	_ = m.save()
}

func (m *ActionCacheMetaManager) RecordTarAccess(cacheDir, includePrefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := SafeFilename(cacheDir)
	entry, exists := m.entries[key]
	if !exists {
		return
	}
	entry.LastAccessAt = time.Now()
	entry.AccessCount++

	if entry.Type == CacheEntryTypeWorkflow && strings.HasPrefix(includePrefix, ".github/workflows/") {
		if entry.WorkflowPath == "" {
			entry.WorkflowPath = includePrefix
			entry.NormalizedUses = buildNormalizedUses(entry.Type, entry.RepoKey, entry.Ref, entry.WorkflowPath)
		}
	}

	_ = m.save()
}

func (m *ActionCacheMetaManager) GetEntries() []*ActionCacheEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*ActionCacheEntry, 0, len(m.entries))
	for _, e := range m.entries {
		result = append(result, e)
	}
	return result
}

func (m *ActionCacheMetaManager) GetEntry(cacheDir string) (*ActionCacheEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := SafeFilename(cacheDir)
	e, ok := m.entries[key]
	return e, ok
}

func (m *ActionCacheMetaManager) GetEntryByExecutionKey(executionKey string) (*ActionCacheEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[executionKey]
	return e, ok
}

func (m *ActionCacheMetaManager) RemoveEntry(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.entries, key)
	_ = m.save()
}

func (m *ActionCacheMetaManager) ScanOrphanEntries() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.scanOrphanEntriesLocked()
}

func (m *ActionCacheMetaManager) scanOrphanEntriesLocked() []string {
	var orphans []string
	entries, err := os.ReadDir(m.path)
	if err != nil {
		return orphans
	}

	metaKeys := make(map[string]bool, len(m.entries))
	for k := range m.entries {
		metaKeys[k] = true
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

func (m *ActionCacheMetaManager) SyncFromDisk() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range m.entries {
		gitPath := filepath.Join(m.path, entry.ExecutionKey+".git")
		if _, err := os.Stat(gitPath); err == nil {
			entry.Size = dirSize(gitPath)
		}
	}
	_ = m.save()
}

func (m *ActionCacheMetaManager) CachePath() string {
	return m.path
}

type ActionCacheWithMeta struct {
	Parent ActionCache
	Meta   *ActionCacheMetaManager
}

func (c *ActionCacheWithMeta) Fetch(ctx context.Context, cacheDir, rawURL, ref, token string) (string, error) {
	sha, err := c.Parent.Fetch(ctx, cacheDir, rawURL, ref, token)
	if err == nil {
		c.Meta.RecordAccess(cacheDir, rawURL, ref, sha)
	}
	return sha, err
}

func (c *ActionCacheWithMeta) GetTarArchive(ctx context.Context, cacheDir, sha, includePrefix string) (io.ReadCloser, error) {
	rc, err := c.Parent.GetTarArchive(ctx, cacheDir, sha, includePrefix)
	if err == nil {
		c.Meta.RecordTarAccess(cacheDir, includePrefix)
	}
	return rc, err
}

func SafeFilename(s string) string {
	return strings.NewReplacer(
		`<`, "-",
		`>`, "-",
		`:`, "-",
		`"`, "-",
		`/`, "-",
		`\`, "-",
		`|`, "-",
		`?`, "-",
		`*`, "-",
	).Replace(s)
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

func FormatEntry(e *ActionCacheEntry) string {
	switch e.Type {
	case CacheEntryTypeWorkflow:
		return fmt.Sprintf("%s (%s @ %s → %s.git) [%s]", e.NormalizedUses, e.RepoSource, e.Ref, e.ExecutionKey, e.Type)
	default:
		return fmt.Sprintf("%s (%s @ %s → %s.git)", e.NormalizedUses, e.RepoSource, e.Ref, e.ExecutionKey)
	}
}
