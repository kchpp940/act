package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseCacheDir(t *testing.T) {
	entryType, repoKey, ref := parseCacheDir("actions/checkout")
	assert.Equal(t, CacheEntryTypeAction, entryType)
	assert.Equal(t, "actions/checkout", repoKey)
	assert.Equal(t, "", ref)

	entryType, repoKey, ref = parseCacheDir("org/repo@v1")
	assert.Equal(t, CacheEntryTypeWorkflow, entryType)
	assert.Equal(t, "org/repo", repoKey)
	assert.Equal(t, "v1", ref)

	entryType, repoKey, ref = parseCacheDir("org/repo@main")
	assert.Equal(t, CacheEntryTypeWorkflow, entryType)
	assert.Equal(t, "org/repo", repoKey)
	assert.Equal(t, "main", ref)
}

func TestBuildNormalizedUses(t *testing.T) {
	assert.Equal(t, "actions/checkout@v3", buildNormalizedUses(CacheEntryTypeAction, "actions/checkout", "v3", ""))
	assert.Equal(t, "actions/checkout", buildNormalizedUses(CacheEntryTypeAction, "actions/checkout", "", ""))

	assert.Equal(t, "org/repo@v1", buildNormalizedUses(CacheEntryTypeWorkflow, "org/repo", "v1", ""))
	assert.Equal(t, "org/repo/.github/workflows/deploy.yml@v1", buildNormalizedUses(CacheEntryTypeWorkflow, "org/repo", "v1", ".github/workflows/deploy.yml"))
}

func TestActionCacheMetaManagerAction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")

	entry, exists := meta.GetEntry("actions/checkout")
	assert.True(t, exists)
	assert.Equal(t, CacheEntryTypeAction, entry.Type)
	assert.Equal(t, "actions/checkout@v3", entry.NormalizedUses)
	assert.Equal(t, "actions/checkout", entry.RepoKey)
	assert.Equal(t, "v3", entry.Ref)
	assert.Equal(t, "abc123", entry.SHA)
	assert.Equal(t, "https://github.com/actions/checkout", entry.RepoURL)
	assert.Equal(t, "github.com", entry.RepoSource)
	assert.Equal(t, "actions-checkout", entry.ExecutionKey)
	assert.Equal(t, "", entry.WorkflowPath)
	assert.Equal(t, 1, entry.AccessCount)
}

func TestActionCacheMetaManagerWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def456")

	entry, exists := meta.GetEntry("org/repo@v1")
	assert.True(t, exists)
	assert.Equal(t, CacheEntryTypeWorkflow, entry.Type)
	assert.Equal(t, "org/repo@v1", entry.NormalizedUses)
	assert.Equal(t, "org/repo", entry.RepoKey)
	assert.Equal(t, "v1", entry.Ref)
	assert.Equal(t, "def456", entry.SHA)
	assert.Equal(t, "https://github.com/org/repo", entry.RepoURL)
	assert.Equal(t, "github.com", entry.RepoSource)
	assert.Equal(t, "org-repo@v1", entry.ExecutionKey)
	assert.Equal(t, "", entry.WorkflowPath)
	assert.Equal(t, 1, entry.AccessCount)
}

func TestActionCacheMetaManagerWorkflowWithPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def456")

	entry, _ := meta.GetEntry("org/repo@v1")
	assert.Equal(t, CacheEntryTypeWorkflow, entry.Type)
	assert.Equal(t, "", entry.WorkflowPath)
	assert.Equal(t, "org/repo@v1", entry.NormalizedUses)

	meta.RecordTarAccess("org/repo@v1", ".github/workflows/deploy.yml")

	entry, _ = meta.GetEntry("org/repo@v1")
	assert.Equal(t, ".github/workflows/deploy.yml", entry.WorkflowPath)
	assert.Equal(t, "org/repo/.github/workflows/deploy.yml@v1", entry.NormalizedUses)
	assert.Equal(t, 2, entry.AccessCount)
}

func TestActionCacheMetaManagerMultipleAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")

	entry, exists := meta.GetEntry("actions/checkout")
	assert.True(t, exists)
	assert.Equal(t, 3, entry.AccessCount)
}

func TestActionCacheMetaManagerPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta1 := NewActionCacheMetaManager(tmpDir)
	meta1.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")

	meta2 := NewActionCacheMetaManager(tmpDir)
	entry, exists := meta2.GetEntry("actions/checkout")
	assert.True(t, exists)
	assert.Equal(t, "actions/checkout@v3", entry.NormalizedUses)
	assert.Equal(t, "actions/checkout", entry.RepoKey)
	assert.Equal(t, "actions-checkout", entry.ExecutionKey)
}

func TestActionCacheMetaManagerPersistenceWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta1 := NewActionCacheMetaManager(tmpDir)
	meta1.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def456")
	meta1.RecordTarAccess("org/repo@v1", ".github/workflows/deploy.yml")

	meta2 := NewActionCacheMetaManager(tmpDir)
	entry, exists := meta2.GetEntry("org/repo@v1")
	assert.True(t, exists)
	assert.Equal(t, CacheEntryTypeWorkflow, entry.Type)
	assert.Equal(t, "org/repo/.github/workflows/deploy.yml@v1", entry.NormalizedUses)
	assert.Equal(t, ".github/workflows/deploy.yml", entry.WorkflowPath)
	assert.Equal(t, "org/repo", entry.RepoKey)
	assert.Equal(t, "org-repo@v1", entry.ExecutionKey)
}

func TestActionCacheMetaManagerSyncFromDiskDoesNotAddOrphans(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "orphan-entry.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	assert.Empty(t, meta.GetEntries())

	meta.SyncFromDisk()
	assert.Empty(t, meta.GetEntries())

	orphans := meta.ScanOrphanEntries()
	assert.Len(t, orphans, 1)
	assert.Equal(t, "orphan-entry", orphans[0])
}

func TestActionCacheMetaManagerSyncFromDiskUpdatesExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")

	testFile := filepath.Join(gitDir, "test.bin")
	err = os.WriteFile(testFile, make([]byte, 1024), 0o644)
	assert.NoError(t, err)

	meta.SyncFromDisk()
	entry, exists := meta.GetEntry("actions/checkout")
	assert.True(t, exists)
	assert.Greater(t, entry.Size, int64(0))
}

func TestActionCacheMetaManagerGetEntryByExecutionKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-meta-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	gitDir2 := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(gitDir2, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc123")
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def456")

	actionEntry, exists := meta.GetEntryByExecutionKey("actions-checkout")
	assert.True(t, exists)
	assert.Equal(t, CacheEntryTypeAction, actionEntry.Type)

	workflowEntry, exists := meta.GetEntryByExecutionKey("org-repo@v1")
	assert.True(t, exists)
	assert.Equal(t, CacheEntryTypeWorkflow, workflowEntry.Type)
}

func TestSafeFilename(t *testing.T) {
	assert.Equal(t, "actions-checkout", SafeFilename("actions/checkout"))
	assert.Equal(t, "org-repo@v1", SafeFilename("org/repo@v1"))
	assert.Equal(t, "a-b-c-d-e", SafeFilename("a/b:c/d*e"))
}

func TestCleanupByMaxAge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "old-action.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("old/action", "https://github.com/example/old-action", "v1", "abc")

	entry, _ := meta.GetEntry("old/action")
	entry.LastAccessAt = time.Now().Add(-48 * time.Hour)
	_ = meta.save()

	cfg := CleanupConfig{MaxAge: 24 * time.Hour}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "old-action", result.RemovedEntries[0].ExecutionKey)
	assert.Equal(t, CacheEntryTypeAction, result.RemovedEntries[0].Type)

	_, err = os.Stat(gitDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupWorkflowByMaxAge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	gitDir2 := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(gitDir2, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "abc")
	meta.RecordTarAccess("org/repo@v1", ".github/workflows/deploy.yml")
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "def")

	workflowEntry, _ := meta.GetEntry("org/repo@v1")
	workflowEntry.LastAccessAt = time.Now().Add(-48 * time.Hour)
	_ = meta.save()

	cfg := CleanupConfig{MaxAge: 24 * time.Hour}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-repo@v1", result.RemovedEntries[0].ExecutionKey)
	assert.Equal(t, CacheEntryTypeWorkflow, result.RemovedEntries[0].Type)
	assert.Equal(t, "org/repo/.github/workflows/deploy.yml@v1", result.RemovedEntries[0].NormalizedUses)

	_, err = os.Stat(gitDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(gitDir2)
	assert.NoError(t, err)
}

func TestCleanupByMinAccessCount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, SafeFilename("rare/action")+".git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	gitDir2 := filepath.Join(tmpDir, SafeFilename("frequent/action")+".git")
	err = os.MkdirAll(gitDir2, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("rare/action", "https://github.com/example/rare-action", "v1", "aaa")
	meta.RecordAccess("frequent/action", "https://github.com/example/frequent-action", "v1", "bbb")
	meta.RecordAccess("frequent/action", "https://github.com/example/frequent-action", "v1", "bbb")
	meta.RecordAccess("frequent/action", "https://github.com/example/frequent-action", "v1", "bbb")

	cfg := CleanupConfig{MinAccessCount: 3}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "rare-action", result.RemovedEntries[0].ExecutionKey)
}

func TestCleanupByRepoSource(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir1 := filepath.Join(tmpDir, SafeFilename("org/ghe-action")+".git")
	err = os.MkdirAll(gitDir1, 0o755)
	assert.NoError(t, err)

	gitDir2 := filepath.Join(tmpDir, SafeFilename("org/gh-action")+".git")
	err = os.MkdirAll(gitDir2, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/ghe-action", "https://ghe.example.com/org/ghe-action", "v1", "aaa")
	meta.RecordAccess("org/gh-action", "https://github.com/org/gh-action", "v1", "bbb")

	cfg := CleanupConfig{RepoSource: "ghe.example.com"}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-ghe-action", result.RemovedEntries[0].ExecutionKey)

	_, err = os.Stat(gitDir1)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(gitDir2)
	assert.NoError(t, err)
}

func TestCleanupByMaxSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir1 := filepath.Join(tmpDir, SafeFilename("big/action")+".git")
	err = os.MkdirAll(gitDir1, 0o755)
	assert.NoError(t, err)
	bigFile := filepath.Join(gitDir1, "data.bin")
	err = os.WriteFile(bigFile, make([]byte, 1024), 0o644)
	assert.NoError(t, err)

	gitDir2 := filepath.Join(tmpDir, SafeFilename("small/action")+".git")
	err = os.MkdirAll(gitDir2, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("big/action", "https://github.com/example/big-action", "v1", "aaa")
	meta.RecordAccess("small/action", "https://github.com/example/small-action", "v1", "bbb")

	entry, _ := meta.GetEntry("big/action")
	entry.LastAccessAt = time.Now().Add(-1 * time.Hour)
	entry2, _ := meta.GetEntry("small/action")
	entry2.LastAccessAt = time.Now()

	cfg := CleanupConfig{MaxSize: 512}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "big-action", result.RemovedEntries[0].ExecutionKey)
}

func TestCleanupDoesNotTouchOrphans(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	managedDir := filepath.Join(tmpDir, SafeFilename("managed/action")+".git")
	err = os.MkdirAll(managedDir, 0o755)
	assert.NoError(t, err)

	orphanDir := filepath.Join(tmpDir, "orphan-action.git")
	err = os.MkdirAll(orphanDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("managed/action", "https://github.com/example/managed-action", "v1", "abc")

	cfg := CleanupConfig{MaxAge: 0}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Empty(t, result.RemovedEntries)

	_, err = os.Stat(orphanDir)
	assert.NoError(t, err)
}

func TestCleanupNoOp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, SafeFilename("keep/action")+".git")
	err = os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("keep/action", "https://github.com/example/keep-action", "v1", "abc")

	cfg := CleanupConfig{MaxAge: 365 * 24 * time.Hour}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Empty(t, result.RemovedEntries)

	_, err = os.Stat(gitDir)
	assert.NoError(t, err)
}

func TestCleanupMixedActionTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	actionGitDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(actionGitDir, 0o755)
	assert.NoError(t, err)

	workflowGitDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(workflowGitDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc")
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def")
	meta.RecordTarAccess("org/repo@v1", ".github/workflows/ci.yml")

	entries := meta.GetEntries()
	assert.Len(t, entries, 2)

	actionEntry, _ := meta.GetEntry("actions/checkout")
	assert.Equal(t, CacheEntryTypeAction, actionEntry.Type)
	assert.Equal(t, "", actionEntry.WorkflowPath)

	workflowEntry, _ := meta.GetEntry("org/repo@v1")
	assert.Equal(t, CacheEntryTypeWorkflow, workflowEntry.Type)
	assert.Equal(t, ".github/workflows/ci.yml", workflowEntry.WorkflowPath)
	assert.Equal(t, "org/repo/.github/workflows/ci.yml@v1", workflowEntry.NormalizedUses)

	actionEntry.LastAccessAt = time.Now().Add(-48 * time.Hour)
	_ = meta.save()

	cfg := CleanupConfig{MaxAge: 24 * time.Hour}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "actions-checkout", result.RemovedEntries[0].ExecutionKey)
	assert.Equal(t, CacheEntryTypeAction, result.RemovedEntries[0].Type)

	_, err = os.Stat(actionGitDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(workflowGitDir)
	assert.NoError(t, err)
}

func TestFormatEntry(t *testing.T) {
	actionEntry := &ActionCacheEntry{
		Type:           CacheEntryTypeAction,
		NormalizedUses: "actions/checkout@v3",
		RepoSource:     "github.com",
		Ref:            "v3",
		ExecutionKey:   "actions-checkout",
	}
	assert.Equal(t, "actions/checkout@v3 (github.com @ v3 → actions-checkout.git)", FormatEntry(actionEntry))

	workflowEntry := &ActionCacheEntry{
		Type:           CacheEntryTypeWorkflow,
		NormalizedUses: "org/repo/.github/workflows/deploy.yml@v1",
		RepoSource:     "github.com",
		Ref:            "v1",
		ExecutionKey:   "org-repo@v1",
		WorkflowPath:   ".github/workflows/deploy.yml",
	}
	assert.Equal(t, "org/repo/.github/workflows/deploy.yml@v1 (github.com @ v1 → org-repo@v1.git) [workflow]", FormatEntry(workflowEntry))
}

func TestInferEntryTypeBackwardCompat(t *testing.T) {
	assert.Equal(t, CacheEntryTypeAction, inferEntryType("actions/checkout"))
	assert.Equal(t, CacheEntryTypeWorkflow, inferEntryType("org/repo@v1"))
	assert.Equal(t, CacheEntryTypeAction, inferEntryType("simple"))
}

func TestMatchesFilters(t *testing.T) {
	action := &ActionCacheEntry{
		Type:         CacheEntryTypeAction,
		Ref:          "v3",
		RepoSource:   "github.com",
		WorkflowPath: "",
	}
	workflow := &ActionCacheEntry{
		Type:         CacheEntryTypeWorkflow,
		Ref:          "v1",
		RepoSource:   "github.com",
		WorkflowPath: ".github/workflows/deploy.yml",
	}

	cfg := CleanupConfig{Type: CacheEntryTypeAction}
	assert.True(t, matchesFilters(action, cfg))
	assert.False(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{Ref: "v3"}
	assert.True(t, matchesFilters(action, cfg))
	assert.False(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{Ref: "v1"}
	assert.False(t, matchesFilters(action, cfg))
	assert.True(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{WorkflowPath: ".github/workflows/deploy"}
	assert.False(t, matchesFilters(action, cfg))
	assert.True(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{WorkflowPath: ".github/workflows/ci"}
	assert.False(t, matchesFilters(action, cfg))
	assert.False(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{RepoSource: "github.com"}
	assert.True(t, matchesFilters(action, cfg))
	assert.True(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{Type: CacheEntryTypeWorkflow, RepoSource: "github.com"}
	assert.False(t, matchesFilters(action, cfg))
	assert.True(t, matchesFilters(workflow, cfg))

	cfg = CleanupConfig{}
	assert.True(t, matchesFilters(action, cfg))
	assert.True(t, matchesFilters(workflow, cfg))
}

func TestCleanupByType(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	actionDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(actionDir, 0o755)
	assert.NoError(t, err)

	workflowDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(workflowDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc")
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def")

	cfg := CleanupConfig{Type: CacheEntryTypeWorkflow}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-repo@v1", result.RemovedEntries[0].ExecutionKey)
	assert.Equal(t, CacheEntryTypeWorkflow, result.RemovedEntries[0].Type)

	_, err = os.Stat(workflowDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(actionDir)
	assert.NoError(t, err)
}

func TestCleanupByRef(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dirV2 := filepath.Join(tmpDir, "actions-checkout-v2.git")
	err = os.MkdirAll(dirV2, 0o755)
	assert.NoError(t, err)

	dirV3 := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(dirV3, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout-v2", "https://github.com/actions/checkout", "v2", "sha2")
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "sha3")

	cfg := CleanupConfig{Ref: "v2"}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "actions-checkout-v2", result.RemovedEntries[0].ExecutionKey)

	_, err = os.Stat(dirV2)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(dirV3)
	assert.NoError(t, err)
}

func TestCleanupByWorkflowPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	deployDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(deployDir, 0o755)
	assert.NoError(t, err)

	ciDir := filepath.Join(tmpDir, "org-repo@v2.git")
	err = os.MkdirAll(ciDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "sha1")
	meta.RecordTarAccess("org/repo@v1", ".github/workflows/deploy.yml")
	meta.RecordAccess("org/repo@v2", "https://github.com/org/repo", "v2", "sha2")
	meta.RecordTarAccess("org/repo@v2", ".github/workflows/ci.yml")

	cfg := CleanupConfig{WorkflowPath: ".github/workflows/deploy"}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-repo@v1", result.RemovedEntries[0].ExecutionKey)

	_, err = os.Stat(deployDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(ciDir)
	assert.NoError(t, err)
}

func TestCleanupCombinedFilters(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	actionDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(actionDir, 0o755)
	assert.NoError(t, err)

	workflowV1Dir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(workflowV1Dir, 0o755)
	assert.NoError(t, err)

	workflowV2Dir := filepath.Join(tmpDir, "org-repo@v2.git")
	err = os.MkdirAll(workflowV2Dir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "abc")
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def")
	meta.RecordAccess("org/repo@v2", "https://github.com/org/repo", "v2", "ghi")

	cfg := CleanupConfig{
		Type: CacheEntryTypeWorkflow,
		Ref:  "v1",
	}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-repo@v1", result.RemovedEntries[0].ExecutionKey)

	_, err = os.Stat(workflowV1Dir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(workflowV2Dir)
	assert.NoError(t, err)

	_, err = os.Stat(actionDir)
	assert.NoError(t, err)
}

func TestCleanupWithFilterAndMaxAge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	recentWorkflowDir := filepath.Join(tmpDir, "org-repo@v1.git")
	err = os.MkdirAll(recentWorkflowDir, 0o755)
	assert.NoError(t, err)

	oldWorkflowDir := filepath.Join(tmpDir, "org-repo@v2.git")
	err = os.MkdirAll(oldWorkflowDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo@v1", "https://github.com/org/repo", "v1", "def")
	meta.RecordAccess("org/repo@v2", "https://github.com/org/repo", "v2", "ghi")

	oldEntry, _ := meta.GetEntry("org/repo@v2")
	oldEntry.LastAccessAt = time.Now().Add(-48 * time.Hour)
	_ = meta.save()

	cfg := CleanupConfig{
		Type:   CacheEntryTypeWorkflow,
		MaxAge: 24 * time.Hour,
	}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 1)
	assert.Equal(t, "org-repo@v2", result.RemovedEntries[0].ExecutionKey)

	_, err = os.Stat(oldWorkflowDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(recentWorkflowDir)
	assert.NoError(t, err)
}

func TestCleanupTypeFilterWithoutOtherCriteriaRemovesAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "act-cache-cleanup-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	workflow1Dir := filepath.Join(tmpDir, "org-repo1@v1.git")
	err = os.MkdirAll(workflow1Dir, 0o755)
	assert.NoError(t, err)

	workflow2Dir := filepath.Join(tmpDir, "org-repo2@v1.git")
	err = os.MkdirAll(workflow2Dir, 0o755)
	assert.NoError(t, err)

	actionDir := filepath.Join(tmpDir, "actions-checkout.git")
	err = os.MkdirAll(actionDir, 0o755)
	assert.NoError(t, err)

	meta := NewActionCacheMetaManager(tmpDir)
	meta.RecordAccess("org/repo1@v1", "https://github.com/org/repo1", "v1", "a")
	meta.RecordAccess("org/repo2@v1", "https://github.com/org/repo2", "v1", "b")
	meta.RecordAccess("actions/checkout", "https://github.com/actions/checkout", "v3", "c")

	cfg := CleanupConfig{Type: CacheEntryTypeWorkflow}
	result, err := CleanupCache(meta, cfg)
	assert.NoError(t, err)
	assert.Len(t, result.RemovedEntries, 2)

	_, err = os.Stat(workflow1Dir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(workflow2Dir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(actionDir)
	assert.NoError(t, err)
}

