package container

import (
	"archive/tar"
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nektos/act/pkg/common"
)

func parseEnvFile(e Container, srcPath string, env *map[string]string) common.Executor {
	localEnv := *env
	return func(ctx context.Context) error {
		envTar, err := e.GetContainerArchive(ctx, srcPath)
		if err != nil {
			return nil
		}
		defer envTar.Close()
		reader := tar.NewReader(envTar)
		_, err = reader.Next()
		if err != nil && err != io.EOF {
			return err
		}
		s := bufio.NewScanner(reader)
		s.Buffer(nil, 1024*1024*1024)
		firstLine := true
		for s.Scan() {
			line := s.Text()
			if firstLine {
				firstLine = false
				if len(line) >= 3 && line[0] == 239 && line[1] == 187 && line[2] == 191 {
					line = line[3:]
				}
			}
			singleLineEnv := strings.Index(line, "=")
			multiLineEnv := strings.Index(line, "<<")
			if singleLineEnv != -1 && (multiLineEnv == -1 || singleLineEnv < multiLineEnv) {
				localEnv[line[:singleLineEnv]] = line[singleLineEnv+1:]
			} else if multiLineEnv != -1 {
				var multiLineEnvContent strings.Builder
				multiLineEnvDelimiter := line[multiLineEnv+2:]
				delimiterFound := false
				firstContentLine := true
				for s.Scan() {
					content := s.Text()
					if content == multiLineEnvDelimiter {
						delimiterFound = true
						break
					}
					if !firstContentLine {
						multiLineEnvContent.WriteByte('\n')
					}
					firstContentLine = false
					multiLineEnvContent.WriteString(content)
				}
				if !delimiterFound {
					return fmt.Errorf("invalid format delimiter '%v' not found before end of file", multiLineEnvDelimiter)
				}
				localEnv[line[:multiLineEnv]] = multiLineEnvContent.String()
			} else {
				return fmt.Errorf("invalid format '%v', expected a line with '=' or '<<'", line)
			}
		}
		env = &localEnv
		return s.Err()
	}
}
