package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
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
	return "Run gogcli commands to interact with Google APIs from the agent, including assisted auth flows."
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
			"account": map[string]interface{}{
				"type":        "string",
				"description": "Optional Google account email used as --account when not explicitly provided in args.",
			},
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Optional stdin payload to send to gogcli (useful for auth codes/prompts).",
			},
			"timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Optional timeout in seconds (default: 90, max: 600).",
				"minimum":     5,
				"maximum":     600,
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

	timeout := t.timeout
	if rawTimeout, ok := args["timeout_seconds"]; ok {
		switch tv := rawTimeout.(type) {
		case float64:
			sec := int(tv)
			if sec >= 5 && sec <= 600 {
				timeout = time.Duration(sec) * time.Second
			}
		case string:
			if sec, err := strconv.Atoi(strings.TrimSpace(tv)); err == nil && sec >= 5 && sec <= 600 {
				timeout = time.Duration(sec) * time.Second
			}
		}
	}

	if !hasAccountArg(argv) {
		if rawAccount, ok := args["account"].(string); ok && strings.TrimSpace(rawAccount) != "" {
			argv = append(argv, "--account", strings.TrimSpace(rawAccount))
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "gogcli", argv...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if rawInput, ok := args["input"].(string); ok && strings.TrimSpace(rawInput) != "" {
		cmd.Stdin = strings.NewReader(rawInput + "\n")
	}

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			combined := mergeOutput(out, errOut)
			if combined == "" {
				return "", fmt.Errorf("gogcli timed out after %v", timeout)
			}
			return combined + "\n\n[notice] gogcli is still waiting for interaction (timed out).", nil
		}
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

func hasAccountArg(argv []string) bool {
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--account" || strings.HasPrefix(argv[i], "--account=") {
			return true
		}
	}
	return false
}

func mergeOutput(out, errOut string) string {
	if out == "" && errOut == "" {
		return ""
	}
	if out == "" {
		return errOut
	}
	if errOut == "" {
		return out
	}
	return out + "\n" + errOut
}
