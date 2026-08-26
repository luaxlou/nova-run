package localsupervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const maxControlMessage = 64 << 10

type controlRequest struct {
	Schema      int    `json:"schema"`
	Action      string `json:"action"`
	ProjectPath string `json:"projectPath"`
	App         string `json:"app"`
	Nonce       string `json:"nonce"`
}

type controlResponse struct {
	Schema int    `json:"schema"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	State  State  `json:"state,omitempty"`
}

type controlAck struct {
	Schema   int  `json:"schema"`
	Received bool `json:"received"`
}

type stopRequest struct {
	response  chan State
	delivered chan struct{}
}

func query(ctx context.Context, paths Paths, expected State) (Status, error) {
	state, err := sendControl(ctx, paths.Socket, controlRequest{
		Schema: StateSchema, Action: "status", ProjectPath: expected.ProjectPath,
		App: expected.App, Nonce: expected.Nonce,
	})
	if err != nil {
		return Status{}, err
	}
	return statusFromState(state), nil
}

func requestStop(ctx context.Context, paths Paths, expected State) error {
	_, err := sendControl(ctx, paths.Socket, controlRequest{
		Schema: StateSchema, Action: "stop", ProjectPath: expected.ProjectPath,
		App: expected.App, Nonce: expected.Nonce,
	})
	return err
}

func sendControl(ctx context.Context, socket string, request controlRequest) (State, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return State{}, fmt.Errorf("connect local supervisor: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return State{}, fmt.Errorf("set local supervisor deadline: %w", err)
		}
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return State{}, fmt.Errorf("send local supervisor request: %w", err)
	}
	var response controlResponse
	if err := json.NewDecoder(io.LimitReader(conn, maxControlMessage)).Decode(&response); err != nil {
		return State{}, fmt.Errorf("read local supervisor response: %w", err)
	}
	if response.Schema != StateSchema {
		return State{}, fmt.Errorf("unsupported local supervisor response schema %d", response.Schema)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "request rejected"
		}
		return State{}, fmt.Errorf("local supervisor: %s", response.Error)
	}
	if request.Action == "stop" {
		if err := json.NewEncoder(conn).Encode(controlAck{Schema: StateSchema, Received: true}); err != nil {
			return State{}, fmt.Errorf("acknowledge local supervisor stop: %w", err)
		}
	}
	return response.State, nil
}

func listenControl(path string) (net.Listener, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale local supervisor socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on local supervisor socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("secure local supervisor socket: %w", err)
	}
	return listener, nil
}

func serveControl(listener net.Listener, current func() State, stops chan<- stopRequest) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go handleControlConnection(conn, current, stops)
	}
}

func handleControlConnection(conn net.Conn, current func() State, stops chan<- stopRequest) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(io.LimitReader(conn, maxControlMessage))
	var request controlRequest
	if err := decoder.Decode(&request); err != nil {
		writeControlResponse(conn, controlResponse{Schema: StateSchema, Error: "invalid control request"})
		return
	}
	state := current()
	if request.Schema != StateSchema || request.ProjectPath != state.ProjectPath || request.App != state.App || request.Nonce != state.Nonce {
		writeControlResponse(conn, controlResponse{Schema: StateSchema, Error: "control identity mismatch"})
		return
	}
	switch request.Action {
	case "status":
		writeControlResponse(conn, controlResponse{Schema: StateSchema, OK: true, State: state})
	case "stop":
		response := make(chan State, 1)
		delivered := make(chan struct{})
		stops <- stopRequest{response: response, delivered: delivered}
		final := <-response
		if err := json.NewEncoder(conn).Encode(controlResponse{Schema: StateSchema, OK: true, State: final}); err == nil {
			var ack controlAck
			_ = decoder.Decode(&ack)
		}
		close(delivered)
	default:
		writeControlResponse(conn, controlResponse{Schema: StateSchema, Error: "unknown control action"})
	}
}

func writeControlResponse(conn net.Conn, response controlResponse) {
	_ = json.NewEncoder(conn).Encode(response)
}
