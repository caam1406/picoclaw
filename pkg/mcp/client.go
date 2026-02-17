package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

type wireMode int

const (
	wireModeLSP wireMode = iota
	wireModeJSONLine
)

type Client struct {
	serverName string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	stderr     io.ReadCloser
	wireMode   wireMode

	writeMu sync.Mutex
	pendMu  sync.Mutex
	pending map[int64]chan rpcResponse

	nextID int64

	closeOnce sync.Once
	closed    chan struct{}
	closeMu   sync.RWMutex
	closeErr  error
}

type RemoteTool struct {
	ServerName  string
	Name        string
	Description string
	InputSchema map[string]interface{}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

func StartClient(ctx context.Context, serverName, command string, args []string, env map[string]string) (*Client, error) {
	const attemptTimeout = 120 * time.Second // uvx may need to download Python + packages on first run

	type result struct {
		client *Client
		mode   wireMode
		err    error
	}

	raceCtx, raceCancel := context.WithTimeout(ctx, attemptTimeout)
	defer raceCancel()

	ch := make(chan result, 2)

	// Try both wire modes concurrently; the first to succeed wins.
	for _, mode := range []wireMode{wireModeLSP, wireModeJSONLine} {
		go func(m wireMode) {
			c, err := startClientWithWireMode(raceCtx, serverName, command, args, env, m)
			ch <- result{client: c, mode: m, err: err}
		}(mode)
	}

	var firstErr error
	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err == nil {
			raceCancel() // signal the other goroutine to stop
			// Drain and close the losing attempt if it arrives later.
			go func() {
				for j := i + 1; j < 2; j++ {
					if loser := <-ch; loser.err == nil && loser.client != nil {
						loser.client.Close()
					}
				}
			}()
			modeName := "LSP"
			if r.mode == wireModeJSONLine {
				modeName = "JSON-line"
			}
			logger.InfoCF("mcp", "MCP client connected", map[string]interface{}{
				"server":    serverName,
				"wire_mode": modeName,
			})
			return r.client, nil
		}
		if firstErr == nil {
			firstErr = r.err
		}
	}

	return nil, fmt.Errorf("all wire modes failed for %q: %w", serverName, firstErr)
}

func startClientWithWireMode(ctx context.Context, serverName, command string, args []string, env map[string]string, mode wireMode) (*Client, error) {
	if strings.TrimSpace(serverName) == "" {
		return nil, fmt.Errorf("serverName is required")
	}
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	cmd := exec.Command(command, args...)

	mergedEnv := append([]string{}, os.Environ()...)
	for k, v := range env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		mergedEnv = append(mergedEnv, k+"="+v)
	}
	cmd.Env = mergedEnv

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command %q: %w", command, err)
	}

	c := &Client{
		serverName: serverName,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     bufio.NewReader(stdoutPipe),
		stderr:     stderrPipe,
		wireMode:   mode,
		pending:    make(map[int64]chan rpcResponse),
		closed:     make(chan struct{}),
	}

	go c.readLoop()
	go c.readStderrLoop()
	go c.waitLoop()

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

func (c *Client) ServerName() string {
	return c.serverName
}

func (c *Client) Close() {
	c.closeWithError(nil)
}

func (c *Client) State() (bool, string) {
	select {
	case <-c.closed:
		c.closeMu.RLock()
		defer c.closeMu.RUnlock()
		if c.closeErr != nil {
			return true, c.closeErr.Error()
		}
		return true, ""
	default:
		return false, ""
	}
}

// Closed returns a channel that is closed when the client terminates.
func (c *Client) Closed() <-chan struct{} {
	return c.closed
}

func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	tools := make([]RemoteTool, 0)
	cursor := ""

	for {
		params := map[string]interface{}{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		raw, err := c.request(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}

		var result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("decode tools/list: %w", err)
		}

		for _, t := range result.Tools {
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			}
			tools = append(tools, RemoteTool{
				ServerName:  c.serverName,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}

		if strings.TrimSpace(result.NextCursor) == "" {
			break
		}
		cursor = strings.TrimSpace(result.NextCursor)
	}

	return tools, nil
}

func (c *Client) CallTool(ctx context.Context, remoteToolName string, args map[string]interface{}) (string, error) {
	raw, err := c.request(ctx, "tools/call", map[string]interface{}{
		"name":      remoteToolName,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Content           []map[string]interface{} `json:"content"`
		StructuredContent interface{}              `json:"structuredContent"`
		IsError           bool                     `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode tools/call result: %w", err)
	}

	parts := make([]string, 0, len(result.Content))
	for _, item := range result.Content {
		t, _ := item["type"].(string)
		if t == "text" {
			if txt, ok := item["text"].(string); ok && strings.TrimSpace(txt) != "" {
				parts = append(parts, txt)
				continue
			}
		}
		b, _ := json.Marshal(item)
		if len(b) > 0 {
			parts = append(parts, string(b))
		}
	}

	var output string
	if len(parts) > 0 {
		output = strings.Join(parts, "\n")
	} else if result.StructuredContent != nil {
		b, _ := json.Marshal(result.StructuredContent)
		output = string(b)
	} else {
		output = "{}"
	}

	if result.IsError {
		return output, fmt.Errorf("mcp tool call returned error")
	}

	return output, nil
}

func (c *Client) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "picoclaw",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize mcp server %q: %w", c.serverName, err)
	}

	_ = c.notify("notifications/initialized", map[string]interface{}{})
	return nil
}

func (c *Client) request(ctx context.Context, method string, params map[string]interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	respCh := make(chan rpcResponse, 1)

	c.pendMu.Lock()
	c.pending[id] = respCh
	c.pendMu.Unlock()

	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := c.writeMessage(msg); err != nil {
		c.removePending(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	case <-c.closed:
		c.removePending(id)
		if c.closeErr != nil {
			return nil, c.closeErr
		}
		return nil, fmt.Errorf("mcp client closed")
	case resp := <-respCh:
		if resp.err != nil {
			return nil, resp.err
		}
		return resp.result, nil
	}
}

func (c *Client) notify(method string, params map[string]interface{}) error {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.writeMessage(msg)
}

func (c *Client) writeMessage(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.wireMode == wireModeLSP {
		if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
			return err
		}
		if _, err := c.stdin.Write(body); err != nil {
			return err
		}
		return nil
	}

	if _, err := c.stdin.Write(body); err != nil {
		return err
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

func (c *Client) readLoop() {
	var decoder *json.Decoder
	if c.wireMode == wireModeJSONLine {
		decoder = json.NewDecoder(c.stdout)
	}

	for {
		var msg rpcMessage
		if c.wireMode == wireModeLSP {
			body, err := readFrame(c.stdout)
			if err != nil {
				if err == io.EOF {
					c.closeWithError(fmt.Errorf("mcp server %q closed stdout", c.serverName))
					return
				}
				c.closeWithError(fmt.Errorf("read frame from %q: %w", c.serverName, err))
				return
			}

			if err := json.Unmarshal(body, &msg); err != nil {
				logger.WarnCF("mcp", "Invalid JSON from MCP server", map[string]interface{}{
					"server": c.serverName,
					"error":  err.Error(),
				})
				continue
			}
		} else {
			if err := decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					c.closeWithError(fmt.Errorf("mcp server %q closed stdout", c.serverName))
					return
				}
				c.closeWithError(fmt.Errorf("decode json-line message from %q: %w", c.serverName, err))
				return
			}
		}

		id, hasID := parseID(msg.ID)
		if !hasID {
			continue
		}

		c.pendMu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.pendMu.Unlock()
		if !ok {
			continue
		}

		if msg.Error != nil {
			ch <- rpcResponse{err: fmt.Errorf("mcp error %d: %s", msg.Error.Code, msg.Error.Message)}
			continue
		}
		ch <- rpcResponse{result: msg.Result}
	}
}

func (c *Client) readStderrLoop() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logger.DebugCF("mcp", "MCP server stderr", map[string]interface{}{
			"server": c.serverName,
			"line":   line,
		})
	}
}

func (c *Client) waitLoop() {
	if err := c.cmd.Wait(); err != nil {
		c.closeWithError(fmt.Errorf("mcp process %q exited: %w", c.serverName, err))
		return
	}
	c.closeWithError(fmt.Errorf("mcp process %q exited", c.serverName))
}

func (c *Client) closeWithError(err error) {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closeErr = err
		c.closeMu.Unlock()
		close(c.closed)

		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}

		c.pendMu.Lock()
		for id, ch := range c.pending {
			delete(c.pending, id)
			if err != nil {
				ch <- rpcResponse{err: err}
			} else {
				ch <- rpcResponse{err: fmt.Errorf("mcp client closed")}
			}
		}
		c.pendMu.Unlock()
	})
}

func (c *Client) removePending(id int64) {
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	delete(c.pending, id)
}

func parseID(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
