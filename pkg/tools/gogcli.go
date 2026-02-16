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
	return "Run gog/gogcli commands to interact with Google APIs, including remote 2-step auth flows."
}

func (t *GoGCLITool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"args": map[string]interface{}{
				"type":        "array",
				"description": "Arguments passed to gog/gogcli, e.g. [\"gmail\", \"search\", \"is:inbox\"].",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Optional high-level action: auth_remote_step1, auth_remote_step2, auth_status.",
			},
			"email": map[string]interface{}{
				"type":        "string",
				"description": "Email used for auth actions.",
			},
			"services": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated services for auth actions (default: user).",
			},
			"auth_url": map[string]interface{}{
				"type":        "string",
				"description": "Full callback URL from browser for auth_remote_step2.",
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
		"required": []string{},
	}
}

func (t *GoGCLITool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	bin, err := resolveGoGBinary()
	if err != nil {
		return "", err
	}

	argv := make([]string, 0)
	if action, _ := args["action"].(string); strings.TrimSpace(action) != "" {
		argv, err = buildActionArgs(args)
		if err != nil {
			return "", err
		}
	} else {
		rawArgs, ok := args["args"].([]interface{})
		if !ok || len(rawArgs) == 0 {
			return "", fmt.Errorf("either action or args is required")
		}
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
	}

	if len(argv) == 0 {
		return "", fmt.Errorf("command args cannot be empty")
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

	cmd := exec.CommandContext(cmdCtx, bin, argv...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if rawInput, ok := args["input"].(string); ok && strings.TrimSpace(rawInput) != "" {
		cmd.Stdin = strings.NewReader(rawInput + "\n")
	}

	err = cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			combined := mergeOutput(out, errOut)
			if combined == "" {
				return "", fmt.Errorf("gog command timed out after %v", timeout)
			}
			return combined + "\n\n[notice] command is still waiting for interaction (timed out).", nil
		}
		if errOut != "" {
			return "", fmt.Errorf("gog command failed: %s", errOut)
		}
		return "", fmt.Errorf("gog command failed: %w", err)
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

func resolveGoGBinary() (string, error) {
	if p, err := exec.LookPath("gog"); err == nil && strings.TrimSpace(p) != "" {
		return "gog", nil
	}
	if p, err := exec.LookPath("gogcli"); err == nil && strings.TrimSpace(p) != "" {
		return "gogcli", nil
	}
	return "", fmt.Errorf("gog binary not found (expected 'gog' or 'gogcli' in PATH)")
}

func buildActionArgs(args map[string]interface{}) ([]string, error) {
	action, _ := args["action"].(string)
	action = strings.TrimSpace(strings.ToLower(action))

	switch action {
	case "auth_status":
		return []string{"auth", "status"}, nil
	case "auth_remote_step1":
		email, _ := args["email"].(string)
		email = strings.TrimSpace(email)
		if email == "" {
			return nil, fmt.Errorf("email is required for auth_remote_step1")
		}
		services, _ := args["services"].(string)
		services = strings.TrimSpace(services)
		if services == "" {
			services = "user"
		}
		return []string{"auth", "add", email, "--services", services, "--remote", "--step", "1"}, nil
	case "auth_remote_step2":
		email, _ := args["email"].(string)
		email = strings.TrimSpace(email)
		if email == "" {
			return nil, fmt.Errorf("email is required for auth_remote_step2")
		}
		authURL, _ := args["auth_url"].(string)
		authURL = strings.TrimSpace(authURL)
		if authURL == "" {
			return nil, fmt.Errorf("auth_url is required for auth_remote_step2")
		}
		services, _ := args["services"].(string)
		services = strings.TrimSpace(services)
		if services == "" {
			services = "user"
		}
		return []string{"auth", "add", email, "--services", services, "--remote", "--step", "2", "--auth-url", authURL}, nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}
