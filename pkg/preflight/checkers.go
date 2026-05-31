package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nektos/act/pkg/container"
)

type dockerDaemonChecker struct {
	socket string
	arch   string
}

func NewDockerDaemonChecker(socket string, containerArch ...string) Checker {
	c := &dockerDaemonChecker{socket: socket}
	if len(containerArch) > 0 {
		c.arch = containerArch[0]
	}
	return c
}

func (c *dockerDaemonChecker) Name() string {
	return "Docker Daemon"
}

func (c *dockerDaemonChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	if c.socket == "-" {
		result.Severity = SeverityWarning
		result.Message = "Docker daemon socket mounting is disabled"
		result.Suggestion = "Container-based actions may not work correctly"
		return result
	}

	dockerHost, _ := container.GetSocketAndHost(c.socket)
	if dockerHost.Host == "" {
		result.Severity = SeverityError
		result.Message = "Docker daemon is not available"
		result.Suggestion = "Start Docker Desktop or ensure DOCKER_HOST is set correctly"
		result.Err = fmt.Errorf("no Docker host found")
		return result
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				host := strings.TrimPrefix(dockerHost.Host, "unix://")
				host = strings.TrimPrefix(host, "npipe://")
				return net.DialTimeout("unix", host, 3*time.Second)
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/_ping", nil)
	if err != nil {
		result.Severity = SeverityError
		result.Message = "Failed to create Docker ping request"
		result.Err = err
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Severity = SeverityError
		result.Message = "Failed to connect to Docker daemon"
		result.Suggestion = "Ensure Docker is running and accessible"
		result.Err = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Docker daemon ping failed with status %d", resp.StatusCode)
		result.Suggestion = "Check Docker daemon health"
		return result
	}

	info, err := container.GetHostInfo(ctx)
	if err != nil {
		result.Severity = SeverityWarning
		result.Message = "Docker daemon is running, but could not retrieve host info"
		result.Err = err
		return result
	}

	messages := []string{fmt.Sprintf("Docker daemon is running (arch: %s, OS: %s)", info.Architecture, info.OSType)}

	if c.arch != "" && info.Architecture != "" {
		requestedArch := c.arch
		if strings.Contains(requestedArch, "/") {
			parts := strings.SplitN(requestedArch, "/", 2)
			requestedArch = parts[1]
		}
		daemonArch := info.Architecture
		if daemonArch == "x86_64" {
			daemonArch = "amd64"
		}
		if daemonArch == "aarch64" {
			daemonArch = "arm64"
		}
		if requestedArch != daemonArch {
			messages = append(messages, fmt.Sprintf("requested architecture %q does not match daemon architecture %q", c.arch, info.Architecture))
			result.Suggestion = "Use --container-architecture matching your Docker daemon, or expect emulation overhead"
		}
	}

	result.Severity = SeverityWarning
	result.Message = strings.Join(messages, "; ")
	return result
}

type workdirChecker struct {
	path string
}

func NewWorkdirChecker(path string) Checker {
	return &workdirChecker{path: path}
}

func (c *workdirChecker) Name() string {
	return "Working Directory"
}

func (c *workdirChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	absPath, err := filepath.Abs(c.path)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to resolve workdir path: %s", c.path)
		result.Err = err
		return result
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Workdir does not exist: %s", absPath)
			result.Suggestion = "Create the directory or specify a valid working directory"
		} else {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Failed to access workdir: %s", absPath)
			result.Err = err
		}
		return result
	}

	if !info.IsDir() {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Workdir is not a directory: %s", absPath)
		result.Suggestion = "Specify a directory path, not a file"
		return result
	}

	tmpFile := filepath.Join(absPath, ".act-write-test-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Workdir is not writable: %s", absPath)
		result.Suggestion = "Check directory permissions"
		result.Err = err
		return result
	}
	os.Remove(tmpFile)

	result.Severity = SeverityWarning
	result.Message = fmt.Sprintf("Workdir is valid and writable: %s", absPath)
	return result
}

type eventFileChecker struct {
	path string
}

func NewEventFileChecker(path string) Checker {
	return &eventFileChecker{path: path}
}

func (c *eventFileChecker) Name() string {
	return "Event File"
}

func (c *eventFileChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	if c.path == "" {
		result.Severity = SeverityWarning
		result.Message = "No event file specified, using default empty event"
		result.Suggestion = "Use --eventpath to specify a custom event JSON file if needed"
		return result
	}

	absPath, err := filepath.Abs(c.path)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to resolve event file path: %s", c.path)
		result.Err = err
		return result
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Event file does not exist: %s", absPath)
			result.Suggestion = "Create the file or verify the path"
		} else {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Failed to access event file: %s", absPath)
			result.Err = err
		}
		return result
	}

	if info.IsDir() {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Event path is a directory, not a file: %s", absPath)
		result.Suggestion = "Specify a JSON file path"
		return result
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to read event file: %s", absPath)
		result.Err = err
		return result
	}

	var eventData interface{}
	if err := json.Unmarshal(content, &eventData); err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Event file is not valid JSON: %s", absPath)
		result.Suggestion = "Ensure the file contains valid JSON"
		result.Err = err
		return result
	}

	result.Severity = SeverityWarning
	result.Message = fmt.Sprintf("Event file is valid: %s", absPath)
	return result
}

type envFilesChecker struct {
	envFile    string
	secretFile string
	varFile    string
}

func NewEnvFilesChecker(envFile, secretFile, varFile string) Checker {
	return &envFilesChecker{
		envFile:    envFile,
		secretFile: secretFile,
		varFile:    varFile,
	}
}

func (c *envFilesChecker) Name() string {
	return "Environment/Secret/Var Files"
}

func (c *envFilesChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	messages := []string{}

	checkFile := func(fileType, path string) {
		if path == "" {
			return
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: failed to resolve path %s: %v", fileType, path, err))
			return
		}

		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				messages = append(messages, fmt.Sprintf("%s: file not found %s", fileType, absPath))
			} else {
				messages = append(messages, fmt.Sprintf("%s: failed to access %s: %v", fileType, absPath, err))
			}
			return
		}

		if info.IsDir() {
			messages = append(messages, fmt.Sprintf("%s: path is a directory, not a file: %s", fileType, absPath))
			return
		}

		if info.Size() == 0 {
			messages = append(messages, fmt.Sprintf("%s: file is empty: %s", fileType, absPath))
		}
	}

	checkFile("env-file", c.envFile)
	checkFile("secret-file", c.secretFile)
	checkFile("var-file", c.varFile)

	if len(messages) > 0 {
		result.Severity = SeverityWarning
		result.Message = strings.Join(messages, "; ")
		result.Suggestion = "Verify the file paths if you intended to use these files"
	} else {
		result.Severity = SeverityWarning
		result.Message = "All environment files are accessible"
	}

	return result
}

type workflowsPathChecker struct {
	path string
}

func NewWorkflowsPathChecker(path string) Checker {
	return &workflowsPathChecker{path: path}
}

func (c *workflowsPathChecker) Name() string {
	return "Workflows Path"
}

func (c *workflowsPathChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	absPath, err := filepath.Abs(c.path)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to resolve workflows path: %s", c.path)
		result.Err = err
		return result
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Workflows path does not exist: %s", absPath)
			result.Suggestion = "Create the directory or specify a valid workflows path"
		} else {
			result.Severity = SeverityError
			result.Message = fmt.Sprintf("Failed to access workflows path: %s", absPath)
			result.Err = err
		}
		return result
	}

	if !info.IsDir() {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Workflows path is not a directory: %s", absPath)
		result.Suggestion = "Specify a directory containing workflow files"
		return result
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to read workflows directory: %s", absPath)
		result.Err = err
		return result
	}

	yamlFiles := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			yamlFiles++
		}
	}

	if yamlFiles == 0 {
		result.Severity = SeverityWarning
		result.Message = fmt.Sprintf("No workflow files found in: %s", absPath)
		result.Suggestion = "Add .yml or .yaml workflow files to the directory"
		return result
	}

	result.Severity = SeverityWarning
	result.Message = fmt.Sprintf("Found %d workflow file(s) in: %s", yamlFiles, absPath)
	return result
}

type actionCacheChecker struct {
	path string
}

func NewActionCacheChecker(path string) Checker {
	return &actionCacheChecker{path: path}
}

func (c *actionCacheChecker) Name() string {
	return "Action Cache Directory"
}

func (c *actionCacheChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	if c.path == "" {
		result.Severity = SeverityWarning
		result.Message = "No action cache path specified, using default"
		return result
	}

	absPath, err := filepath.Abs(c.path)
	if err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to resolve action cache path: %s", c.path)
		result.Err = err
		return result
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(absPath, 0o755); err != nil {
				result.Severity = SeverityError
				result.Message = fmt.Sprintf("Failed to create action cache directory: %s", absPath)
				result.Suggestion = "Check parent directory permissions"
				result.Err = err
				return result
			}
			result.Severity = SeverityWarning
			result.Message = fmt.Sprintf("Created action cache directory: %s", absPath)
			return result
		}
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Failed to access action cache path: %s", absPath)
		result.Err = err
		return result
	}

	if !info.IsDir() {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Action cache path is not a directory: %s", absPath)
		result.Suggestion = "Specify a directory path"
		return result
	}

	tmpFile := filepath.Join(absPath, ".write-test-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("Action cache directory is not writable: %s", absPath)
		result.Suggestion = "Check directory permissions"
		result.Err = err
		return result
	}
	os.Remove(tmpFile)

	result.Severity = SeverityWarning
	result.Message = fmt.Sprintf("Action cache directory is valid: %s", absPath)
	return result
}

type platformChecker struct {
	platforms   map[string]string
	arch        string
}

func NewPlatformChecker(platforms map[string]string, arch string) Checker {
	return &platformChecker{
		platforms:   platforms,
		arch:        arch,
	}
}

func (c *platformChecker) Name() string {
	return "Platform Configuration"
}

func (c *platformChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	if len(c.platforms) == 0 {
		result.Severity = SeverityError
		result.Message = "No platforms configured"
		result.Suggestion = "Run `act` to configure default platforms or use -P flag"
		return result
	}

	errors := []string{}
	warnings := []string{}

	for platform, image := range c.platforms {
		if image == "" {
			errors = append(errors, fmt.Sprintf("%s: no image configured", platform))
			continue
		}
		parts := strings.SplitN(image, ":", 2)
		if len(parts) < 2 || parts[1] == "" {
			warnings = append(warnings, fmt.Sprintf("%s: image %q has no tag specified, will use latest", platform, image))
		}
		if strings.Contains(parts[0], " ") {
			errors = append(errors, fmt.Sprintf("%s: invalid image name %q", platform, image))
		}
	}

	if c.arch != "" {
		validArchs := map[string]bool{
			"linux/amd64": true,
			"linux/arm64": true,
			"linux/arm":   true,
			"amd64":       true,
			"arm64":       true,
			"arm":         true,
		}
		if !validArchs[c.arch] {
			errors = append(errors, fmt.Sprintf("Unknown container architecture: %s", c.arch))
		}
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && c.arch == "" {
		warnings = append(warnings, "Apple Silicon detected without explicit container architecture")
		result.Suggestion = "Consider using --container-architecture linux/amd64 for better compatibility"
	}

	if len(errors) > 0 {
		result.Severity = SeverityError
		result.Message = strings.Join(errors, "; ")
	} else if len(warnings) > 0 {
		result.Severity = SeverityWarning
		result.Message = strings.Join(warnings, "; ")
	} else {
		result.Severity = SeverityWarning
		result.Message = fmt.Sprintf("Platforms configured: %d platform(s)", len(c.platforms))
		if c.arch != "" {
			result.Message += fmt.Sprintf(" (architecture: %s)", c.arch)
		}
	}

	return result
}

type plannerResultChecker struct {
	plan      PlanInfo
	eventName string
}

func NewPlannerResultChecker(plan PlanInfo, eventName string) Checker {
	return &plannerResultChecker{
		plan:      plan,
		eventName: eventName,
	}
}

func (c *plannerResultChecker) Name() string {
	return "Workflow Selection"
}

func (c *plannerResultChecker) Check(ctx context.Context) CheckResult {
	result := CheckResult{
		Name: c.Name(),
	}

	if c.plan.IsEmpty {
		result.Severity = SeverityError
		result.Message = fmt.Sprintf("No jobs to run for event %q", c.eventName)
		result.Suggestion = "Use `act --list` to view available jobs, or specify a different event/job with positional argument or --job flag"
		return result
	}

	if c.plan.TotalJobs == 0 {
		result.Severity = SeverityError
		result.Message = "Plan contains no jobs"
		result.Suggestion = "Check your workflow files for syntax errors or empty job definitions"
		return result
	}

	jobList := strings.Join(c.plan.JobIDs, ", ")
	if len(jobList) > 80 {
		jobList = jobList[:77] + "..."
	}

	result.Severity = SeverityWarning
	result.Message = fmt.Sprintf("Event %q: %d stage(s), %d job(s) [%s]", c.eventName, c.plan.Stages, c.plan.TotalJobs, jobList)
	return result
}
