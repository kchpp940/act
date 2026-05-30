package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	goURL "net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/filecollector"
)

type LocalRepositoryCache struct {
	Parent            ActionCache
	LocalRepositories map[string]string
	CacheDirCache     map[string]string
}

func (l *LocalRepositoryCache) Fetch(ctx context.Context, source *ActionSource) (string, error) {
	logger := common.Logger(ctx)
	logger.Debugf("LocalRepositoryCache fetch %s with ref %s", source.CloneURL, source.Ref)
	if dest, ok := l.LocalRepositories[fmt.Sprintf("%s@%s", source.CloneURL, source.Ref)]; ok {
		logger.Infof("LocalRepositoryCache matched %s with ref %s to %s", source.CloneURL, source.Ref, dest)
		l.CacheDirCache[fmt.Sprintf("%s@%s", source.CacheDir, source.Ref)] = dest
		return source.Ref, nil
	}
	if purl, err := goURL.Parse(source.CloneURL); err == nil {
		if dest, ok := l.LocalRepositories[fmt.Sprintf("%s@%s", strings.TrimPrefix(purl.Path, "/"), source.Ref)]; ok {
			logger.Infof("LocalRepositoryCache matched %s with ref %s to %s", source.CloneURL, source.Ref, dest)
			l.CacheDirCache[fmt.Sprintf("%s@%s", source.CacheDir, source.Ref)] = dest
			return source.Ref, nil
		}
	}
	logger.Infof("LocalRepositoryCache not matched %s with Ref %s", source.CloneURL, source.Ref)
	return l.Parent.Fetch(ctx, source)
}

func (l *LocalRepositoryCache) GetTarArchive(ctx context.Context, source *ActionSource, sha, includePrefix string) (io.ReadCloser, error) {
	logger := common.Logger(ctx)
	if dest, ok := l.CacheDirCache[fmt.Sprintf("%s@%s", source.CacheDir, sha)]; ok {
		logger.Infof("LocalRepositoryCache read cachedir %s with ref %s and subpath '%s' from %s", source.CacheDir, sha, includePrefix, dest)
		srcPath := filepath.Join(dest, includePrefix)
		buf := &bytes.Buffer{}
		tw := tar.NewWriter(buf)
		defer tw.Close()
		srcPath = filepath.Clean(srcPath)
		fi, err := os.Lstat(srcPath)
		if err != nil {
			return nil, err
		}
		tc := &filecollector.TarCollector{
			TarWriter: tw,
		}
		if fi.IsDir() {
			srcPrefix := srcPath
			if !strings.HasSuffix(srcPrefix, string(filepath.Separator)) {
				srcPrefix += string(filepath.Separator)
			}
			fc := &filecollector.FileCollector{
				Fs:        &filecollector.DefaultFs{},
				SrcPath:   srcPath,
				SrcPrefix: srcPrefix,
				Handler:   tc,
			}
			err = filepath.Walk(srcPath, fc.CollectFiles(ctx, []string{}))
			if err != nil {
				return nil, err
			}
		} else {
			var f io.ReadCloser
			var linkname string
			if fi.Mode()&fs.ModeSymlink != 0 {
				linkname, err = os.Readlink(srcPath)
				if err != nil {
					return nil, err
				}
			} else {
				f, err = os.Open(srcPath)
				if err != nil {
					return nil, err
				}
				defer f.Close()
			}
			err := tc.WriteFile(fi.Name(), fi, linkname, f)
			if err != nil {
				return nil, err
			}
		}
		return io.NopCloser(buf), nil
	}
	logger.Infof("LocalRepositoryCache not matched cachedir %s with Ref %s and subpath '%s'", source.CacheDir, sha, includePrefix)
	return l.Parent.GetTarArchive(ctx, source, sha, includePrefix)
}
