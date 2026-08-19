package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/pkg/api"
)

type Client struct {
	Endpoint string
	Token    string
}

func NewClient() *Client {
	endpoint := os.Getenv("NOVA_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("NOVA_AGENT_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = "http://127.0.0.1:32102"
	}
	token := os.Getenv("NOVA_TOKEN")
	if token == "" {
		token = os.Getenv("NOVA_AGENT_TOKEN")
	}
	return &Client{
		Endpoint: endpoint,
		Token:    token,
	}
}

func (c *Client) authHeader() string {
	if c.Token == "" {
		return ""
	}
	return "Bearer " + c.Token
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) ([]byte, error) {
	endpoint := strings.TrimRight(c.Endpoint, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", c.authHeader())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(payload))
		if msg == "" {
			msg = resp.Status
		}
		return payload, fmt.Errorf("http %d: %s", resp.StatusCode, msg)
	}
	return payload, nil
}

func (c *Client) doRequestStream(ctx context.Context, method, path string, body io.Reader, contentType string, sink io.Writer) error {
	endpoint := strings.TrimRight(c.Endpoint, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", c.authHeader())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(payload))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, msg)
	}
	if _, err := io.Copy(sink, resp.Body); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	return nil
}

func (c *Client) parseResponse(payload []byte, out any) error {
	if len(payload) == 0 {
		return nil
	}
	var resp api.Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !resp.Success {
		if resp.Message == "" {
			return fmt.Errorf("request not successful")
		}
		return fmt.Errorf(resp.Message)
	}
	if out == nil {
		return nil
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		return fmt.Errorf("marshal response data: %w", err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unmarshal response data: %w", err)
	}
	return nil
}

func (c *Client) deployArtifact(ctx context.Context, name string, artifactReader io.Reader) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	if artifactReader == nil {
		return fmt.Errorf("artifact stream required")
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("artifact", "artifact.tar.gz")
	if err != nil {
		return fmt.Errorf("create multipart form: %w", err)
	}
	if _, err := io.Copy(part, artifactReader); err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("close multipart form: %w", err)
	}

	payload, err := c.doRequest(ctx, http.MethodPut, "/v1/apps/"+url.PathEscape(name), &body, mw.FormDataContentType())
	if err != nil {
		return err
	}
	if err := c.parseResponse(payload, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) Deploy(ctx context.Context, name, artifactDir string) error {
	if artifactDir == "" {
		return fmt.Errorf("artifact dir required")
	}
	if info, err := os.Stat(artifactDir); err != nil || !info.IsDir() {
		return fmt.Errorf("artifact dir invalid: %w", err)
	}
	runPath := filepath.Join(artifactDir, "run")
	if _, err := os.Stat(runPath); err != nil {
		return fmt.Errorf("artifact must contain run: %w", err)
	}
	tmp, err := os.CreateTemp("", "nova-artifact-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temporary archive: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := artifact.PackDir(artifactDir, tmpPath); err != nil {
		return fmt.Errorf("pack artifact: %w", err)
	}

	stream, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open artifact archive: %w", err)
	}
	defer stream.Close()

	return c.deployArtifact(ctx, name, stream)
}

func (c *Client) Start(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	payload, err := c.doRequest(ctx, http.MethodPost, "/v1/apps/"+url.PathEscape(name)+"/start", nil, "")
	if err != nil {
		return err
	}
	return c.parseResponse(payload, nil)
}

func (c *Client) Stop(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	payload, err := c.doRequest(ctx, http.MethodPost, "/v1/apps/"+url.PathEscape(name)+"/stop", nil, "")
	if err != nil {
		return err
	}
	return c.parseResponse(payload, nil)
}

func (c *Client) Restart(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	payload, err := c.doRequest(ctx, http.MethodPost, "/v1/apps/"+url.PathEscape(name)+"/restart", nil, "")
	if err != nil {
		return err
	}
	return c.parseResponse(payload, nil)
}

func (c *Client) Status(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("app name required")
	}
	payload, err := c.doRequest(ctx, http.MethodGet, "/v1/apps/"+url.PathEscape(name)+"/status", nil, "")
	if err != nil {
		return "", err
	}

	var status struct {
		State    string `json:"state"`
		SubState string `json:"subState"`
		PID      string `json:"pid"`
		Started  string `json:"started"`
		ExitCode string `json:"exitCode"`
	}
	if err := c.parseResponse(payload, &status); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"app=%s state=%s sub=%s pid=%s started=%s exit=%s",
		name, status.State, status.SubState, status.PID, status.Started, status.ExitCode,
	), nil
}

func (c *Client) Logs(ctx context.Context, name string, follow bool) ([]string, error) {
	if follow {
		return nil, fmt.Errorf("follow mode should use LogsStream")
	}
	if name == "" {
		return nil, fmt.Errorf("app name required")
	}
	path := "/v1/apps/" + url.PathEscape(name) + "/logs"
	payload, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	var lines []string
	if err := c.parseResponse(payload, &lines); err != nil {
		var data string
		if err := c.parseResponse(payload, &data); err != nil {
			return nil, err
		}
		if strings.TrimSpace(data) != "" {
			lines = []string{data}
		}
		return lines, nil
	}
	return lines, nil
}

func (c *Client) LogsStream(ctx context.Context, name string, out io.Writer) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	if out == nil {
		return fmt.Errorf("output writer required")
	}
	path := "/v1/apps/" + url.PathEscape(name) + "/logs?follow=true"
	return c.doRequestStream(ctx, http.MethodGet, path, nil, "", out)
}

func (c *Client) List(ctx context.Context) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context required")
	}
	payload, err := c.doRequest(ctx, http.MethodGet, "/v1/apps", nil, "")
	if err != nil {
		return nil, err
	}
	var items []string
	if err := c.parseResponse(payload, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) Remove(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	payload, err := c.doRequest(ctx, http.MethodDelete, "/v1/apps/"+url.PathEscape(name), nil, "")
	if err != nil {
		return err
	}
	return c.parseResponse(payload, nil)
}
