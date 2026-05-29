package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:gosec
func TestActionCache(t *testing.T) {
	a := assert.New(t)
	cache := &GoGitActionCache{
		Path: os.TempDir(),
	}
	ctx := context.Background()
	refs := []struct {
		Name  string
		Uses  string
		Ar    *actionRef
		SubPath string
	}{
		{
			Name:    "Fetch Branch Name",
			Uses:    "nektos/act-test-actions@main",
			SubPath: "js",
		},
		{
			Name:    "Fetch Branch Name Absolutely",
			Uses:    "nektos/act-test-actions@refs/heads/main",
			SubPath: "js",
		},
		{
			Name:    "Fetch HEAD",
			Uses:    "nektos/act-test-actions@HEAD",
			SubPath: "js",
		},
		{
			Name:    "Fetch Sha",
			Uses:    "nektos/act-test-actions@de984ca37e4df4cb9fd9256435a3b82c4a2662b1",
			SubPath: "js",
		},
	}
	for _, c := range refs {
		t.Run(c.Name, func(_ *testing.T) {
			ar := newRemoteActionRef(c.Uses, nil, &Config{})
			if !a.NotNil(ar) {
				return
			}
			sha, err := cache.Fetch(ctx, ar)
			if !a.NoError(err) || !a.NotEmpty(sha) {
				return
			}
			atar, err := cache.GetTarArchive(ctx, ar.RepoCacheKey(), sha, c.SubPath)
			if !a.NoError(err) || !a.NotEmpty(atar) {
				return
			}
			mytar := tar.NewReader(atar)
			th, err := mytar.Next()
			if !a.NoError(err) || !a.NotEqual(0, th.Size) {
				return
			}
			buf := &bytes.Buffer{}
			// G110: Potential DoS vulnerability via decompression bomb (gosec)
			_, err = io.Copy(buf, mytar)
			a.NoError(err)
			str := buf.String()
			a.NotEmpty(str)
		})
	}
}

func TestActionCacheFailures(t *testing.T) {
	a := assert.New(t)
	cache := &GoGitActionCache{
		Path: os.TempDir(),
	}
	ctx := context.Background()
	refs := []struct {
		Name string
		Uses string
	}{
		{
			Name: "Fetch Branch Name",
			Uses: "nektos/act-test-actions-not-exist@main",
		},
		{
			Name: "Fetch Branch Name Absolutely",
			Uses: "nektos/act-test-actions-not-exist@refs/heads/main",
		},
		{
			Name: "Fetch HEAD",
			Uses: "nektos/act-test-actions-not-exist@HEAD",
		},
		{
			Name: "Fetch Sha",
			Uses: "nektos/act-test-actions-not-exist@de984ca37e4df4cb9fd9256435a3b82c4a2662b1",
		},
		{
			Name: "Fetch Branch Name no existing",
			Uses: "nektos/act-test-actions@main2",
		},
		{
			Name: "Fetch Branch Name Absolutely no existing",
			Uses: "nektos/act-test-actions@refs/heads/main2",
		},
		{
			Name: "Fetch Sha no existing",
			Uses: "nektos/act-test-actions@de984ca37e4df4cb9fd9256435a3b82c4a2662b2",
		},
	}
	for _, c := range refs {
		t.Run(c.Name, func(t *testing.T) {
			ar := newRemoteActionRef(c.Uses, nil, &Config{})
			if !a.NotNil(ar) {
				return
			}
			_, err := cache.Fetch(ctx, ar)
			t.Logf("%s\n", err)
			if !a.Error(err) {
				return
			}
		})
	}
}
