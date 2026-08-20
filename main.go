package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	AllowedPrograms    []string `json:"allowed_programs"`
	AllowedDirectories []string `json:"allowed_directories"`
	TimeoutSeconds     int      `json:"timeout_seconds"`
}

type RunInput struct {
	Program string   `json:"program" jsonschema:"Executable name to run, such as docker, git, hostname, or whoami"`
	Args    []string `json:"args,omitempty" jsonschema:"Arguments passed directly to the executable"`
	Cwd     string   `json:"cwd,omitempty" jsonschema:"Optional working directory; must be inside an allowed directory"`
}

type RunOutput struct {
	OS       string `json:"os"`
	Command  string `json:"command"`
	Cwd      string `json:"cwd,omitempty"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

var config Config

func defaultConfig() Config {
	return Config{
		AllowedPrograms: []string{
			"hostname",
			"whoami",
			"git",
			"docker",
			"go",
			"php",
			"ping",
			"ipconfig",
			"ip",
			"df",
			"free",
			"uptime",
			"ls",
		},
		AllowedDirectories: nil,
		TimeoutSeconds:     30,
	}
}

func loadConfig() Config {
	cfg := defaultConfig()

	path := os.Getenv("MCP_SYSTEM_AGENT_CONFIG")
	if path == "" {
		path = "config.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("warning: could not read config %q: %v", path, err)
		}
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("warning: invalid config %q: %v; using defaults", path, err)
		return defaultConfig()
	}

	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}

	return cfg
}

func normalizedProgram(program string) (string, error) {
	program = strings.TrimSpace(program)
	if program == "" {
		return "", fmt.Errorf("program is required")
	}

	if filepath.Base(program) != program || strings.ContainsAny(program, `/\\`) {
		return "", fmt.Errorf("program must be an executable name, not a path")
	}

	name := strings.ToLower(program)
	name = strings.TrimSuffix(name, ".exe")
	return name, nil
}

func programAllowed(program string) bool {
	name, err := normalizedProgram(program)
	if err != nil {
		return false
	}

	for _, allowed := range config.AllowedPrograms {
		allowedName := strings.ToLower(strings.TrimSpace(allowed))
		allowedName = strings.TrimSuffix(allowedName, ".exe")
		if name == allowedName {
			return true
		}
	}

	return false
}

func pathInside(path, root string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}

	if resolved, err := filepath.EvalSymlinks(pathAbs); err == nil {
		pathAbs = resolved
	}
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = resolved
	}

	if runtime.GOOS == "windows" {
		pathAbs = strings.ToLower(pathAbs)
		rootAbs = strings.ToLower(rootAbs)
	}

	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func validateCwd(cwd string) error {
	if cwd == "" {
		return nil
	}

	if len(config.AllowedDirectories) == 0 {
		return fmt.Errorf("working directory use is disabled until allowed_directories is configured")
	}

	info, err := os.Stat(cwd)
	if err != nil {
		return fmt.Errorf("working directory is not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory")
	}

	for _, root := range config.AllowedDirectories {
		if pathInside(cwd, root) {
			return nil
		}
	}

	return fmt.Errorf("working directory is outside allowed_directories")
}

func runCommand(ctx context.Context, req *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, RunOutput, error) {
	if _, err := normalizedProgram(input.Program); err != nil {
		return nil, RunOutput{}, err
	}

	if !programAllowed(input.Program) {
		return nil, RunOutput{
			OS:       runtime.GOOS,
			Command:  input.Program,
			Cwd:      input.Cwd,
			Output:   "Program blocked by policy",
			ExitCode: -1,
		}, nil
	}

	if err := validateCwd(input.Cwd); err != nil {
		return nil, RunOutput{
			OS:       runtime.GOOS,
			Command:  input.Program,
			Cwd:      input.Cwd,
			Output:   err.Error(),
			ExitCode: -1,
		}, nil
	}

	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, input.Program, input.Args...)
	if input.Cwd != "" {
		cmd.Dir = input.Cwd
	}

	output, err := cmd.CombinedOutput()
	exitCode := 0

	if cmdCtx.Err() == context.DeadlineExceeded {
		exitCode = -1
		output = append(output, []byte("\nCommand timed out")...)
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			if len(output) == 0 {
				output = []byte(err.Error())
			}
		}
	}

	commandText := input.Program
	if len(input.Args) > 0 {
		commandText += " " + strings.Join(input.Args, " ")
	}

	return nil, RunOutput{
		OS:       runtime.GOOS,
		Command:  commandText,
		Cwd:      input.Cwd,
		Output:   string(output),
		ExitCode: exitCode,
	}, nil
}

func main() {
	config = loadConfig()

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-system-agent",
			Version: "v0.1.0",
		},
		nil,
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "run_command",
			Description: "Run an approved executable directly on the local Windows or Linux host without invoking a shell.",
		},
		runCommand,
	)

	log.Printf("mcp-system-agent v0.1.0 starting on %s/%s", runtime.GOOS, runtime.GOARCH)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
