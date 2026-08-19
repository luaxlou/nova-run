package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Client struct {
	Endpoint string
	Token    string
}

func NewClient() *Client {
	endpoint := os.Getenv("NOVA_AGENT_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:32102"
	}
	return &Client{
		Endpoint: endpoint,
		Token:    os.Getenv("NOVA_AGENT_TOKEN"),
	}
}

func (c *Client) authHeader() string {
	return "Bearer " + c.Token
}

func (c *Client) deployArtifact(_ context.Context, name string, _ io.Reader) error {
	if name == "" {
		return fmt.Errorf("app name required")
	}
	return nil
}

func (c *Client) Deploy(ctx context.Context, name, artifactDir string) error {
	_ = ctx
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
	return c.deployArtifact(context.Background(), name, nil)
}

func (c *Client) Start(ctx context.Context, name string) error {
	_ = c
	_ = ctx
	if name == "" {
		return fmt.Errorf("app name required")
	}
	return nil
}

func (c *Client) Stop(ctx context.Context, name string) error {
	_ = c
	_ = ctx
	if name == "" {
		return fmt.Errorf("app name required")
	}
	return nil
}

func (c *Client) Restart(ctx context.Context, name string) error {
	_ = c
	_ = ctx
	if name == "" {
		return fmt.Errorf("app name required")
	}
	return nil
}

func (c *Client) Status(ctx context.Context, name string) (string, error) {
	_ = c
	_ = ctx
	if name == "" {
		return "", fmt.Errorf("app name required")
	}
	return fmt.Sprintf("app %s status: unknown in this milestone", name), nil
}

func (c *Client) Logs(ctx context.Context, name string, follow bool) (string, error) {
	_ = c
	_ = ctx
	if name == "" {
		return "", fmt.Errorf("app name required")
	}
	if follow {
		return "logs stream (not implemented yet)", nil
	}
	return "logs snapshot (not implemented yet)", nil
}

func (c *Client) List(ctx context.Context) ([]string, error) {
	_ = c
	_ = ctx
	return nil, nil
}

func (c *Client) Remove(ctx context.Context, name string) error {
	_ = c
	_ = ctx
	if name == "" {
		return fmt.Errorf("app name required")
	}
	return nil
}

