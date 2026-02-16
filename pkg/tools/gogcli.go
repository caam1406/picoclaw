package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type GoGCLITool struct {
	timeout time.Duration
}

func NewGoGCLITool() *GoGCLITool {
	return &GoGCLITool{
		timeout: 90 * time.Second,
	}
}

func (t *GoGCLITool) Name() string {
	return "gogcli"
}

func (t *GoGCLITool) Description() string {
	return "Run gogcli commands to interact with Google APIs from the agent."
}

func (t *GoGCLITool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"args": map[string]interface{}{
				"type":        "array",
				"description": "Arguments passed to gogcli, e.g. [\"gmail\", \"list\", \"--max\", \"5\"].",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"args"},
	}
}

func (t *GoGCLITool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	rawArgs, ok := args["args"].([]interface{})
	if !ok || len(rawArgs) == 0 {
		return "", fmt.Errorf("args is required and must be a non-empty array")
	}

	argv := make([]string, 0, len(rawArgs))
	for _, item := range rawArgs {
		s, ok := item.(string)
		if !ok {
			return "", fmt.Errorf("args must contain only strings")
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		argv = append(argv, s)
	}

	if len(argv) == 0 {
		return "", fmt.Errorf("args cannot be empty")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "gogcli", argv...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())

	if cmdCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("gogcli timed out after %v", t.timeout)
	}

	if err != nil {
		if errOut != "" {
			return "", fmt.Errorf("gogcli failed: %s", errOut)
		}
		return "", fmt.Errorf("gogcli failed: %w", err)
	}

	if out == "" && errOut != "" {
		return errOut, nil
	}
	if out == "" {
		return "(no output)", nil
	}
	return out, nil
}

