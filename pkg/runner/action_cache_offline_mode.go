package runner

import (
	"context"
	"io"
	"path"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/nektos/act/pkg/common"
)

type GoGitActionCacheOfflineMode struct {
	Parent GoGitActionCache
}

func (c GoGitActionCacheOfflineMode) Fetch(ctx context.Context, source *ActionSource) (string, error) {
	logger := common.Logger(ctx)

	gitPath := path.Join(c.Parent.Path, safeFilename(source.CacheDir)+".git")

	logger.Infof("GoGitActionCacheOfflineMode fetch content %s with ref %s at %s", source.CloneURL, source.Ref, gitPath)

	sha, fetchErr := c.Parent.Fetch(ctx, source)
	gogitrepo, err := git.PlainOpen(gitPath)
	if err != nil {
		return "", fetchErr
	}
	refName := plumbing.ReferenceName("refs/action-cache-offline/" + source.Ref)
	r, err := gogitrepo.Reference(refName, true)
	if fetchErr == nil {
		if err != nil || sha != r.Hash().String() {
			if err == nil {
				refName = r.Name()
			}
			ref := plumbing.NewHashReference(refName, plumbing.NewHash(sha))
			_ = gogitrepo.Storer.SetReference(ref)
		}
	} else if err == nil {
		return r.Hash().String(), nil
	}
	return sha, fetchErr
}

func (c GoGitActionCacheOfflineMode) GetTarArchive(ctx context.Context, source *ActionSource, sha, includePrefix string) (io.ReadCloser, error) {
	return c.Parent.GetTarArchive(ctx, source, sha, includePrefix)
}
