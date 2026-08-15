// Package sdk is the Go binder for redstone — deliberately micro: bind,
// optional, edge identity. If this file grows features, it is failing
// (frameworks are bricks, not SDK residents).
package sdk

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	corev1 "github.com/tomblancdev/redstone/gen/redstone/core/v1"
)

// Client binds capabilities for one app in one stack.
type Client struct {
	conn  *grpc.ClientConn
	stub  corev1.RegisterServiceClient
	stack string
	app   string
}

// Binding is a powered circuit: where to call, and who you are on the edge.
type Binding struct {
	Name           string
	Capability     string
	EffectiveLevel string
	Verified       bool
	Endpoint       string
	Public         string
	Task           string
	edge           string
}

// Header returns the edge identity header to send on capability calls, so
// shared adapters can pull this edge's with/wire config from the register.
func (b *Binding) Header() (key, value string) { return "X-Edge", b.edge }

// Dial creates a client. The connection is lazy — no network until Bind.
func Dial(register, stack, app string) (*Client, error) {
	conn, err := grpc.NewClient(register, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, stub: corev1.NewRegisterServiceClient(conn), stack: stack, app: app}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Opt tweaks a bind request. Zero opts = declaration-driven: capability and
// level come from the stack file, the single source of truth.
type Opt func(*corev1.BindRequest)

func WithCapability(capability, level string) Opt {
	return func(r *corev1.BindRequest) { r.Capability, r.Level = capability, level }
}
func WithName(name string) Opt { return func(r *corev1.BindRequest) { r.Name = name } }
func WithLabels(labels map[string]string) Opt {
	return func(r *corev1.BindRequest) { r.Labels = labels }
}

// Bind resolves a task through the register. Refusals return an error
// carrying the register's written reasons.
func (c *Client) Bind(ctx context.Context, task string, opts ...Opt) (*Binding, error) {
	req := &corev1.BindRequest{Stack: c.stack, Consumer: c.app, As: task}
	for _, opt := range opts {
		opt(req)
	}
	resp, err := c.stub.Bind(ctx, req)
	if err != nil {
		if s, ok := status.FromError(err); ok {
			// FAILED_PRECONDITION details carry the refusal JSON verbatim.
			var refusal struct {
				Error      string                          `json:"error"`
				Candidates []struct{ Name, Reason string } `json:"candidates"`
			}
			if json.Unmarshal([]byte(s.Message()), &refusal) == nil && refusal.Error != "" {
				return nil, fmt.Errorf("bind %s/%s.%s: %s %v", c.stack, c.app, task, refusal.Error, refusal.Candidates)
			}
			return nil, fmt.Errorf("bind %s/%s.%s: %s", c.stack, c.app, task, s.Message())
		}
		return nil, err
	}
	return &Binding{
		Name:           resp.Name,
		Capability:     resp.Capability,
		EffectiveLevel: resp.EffectiveLevel,
		Verified:       resp.Verified,
		Endpoint:       resp.Endpoint,
		Public:         resp.Public,
		Task:           task,
		edge:           fmt.Sprintf("%s/%s/%s", c.stack, c.app, task),
	}, nil
}

// Optional binds a task the app can live without: unresolved returns nil,
// never an error — run with the feature off.
func (c *Client) Optional(ctx context.Context, task string, opts ...Opt) *Binding {
	b, err := c.Bind(ctx, task, opts...)
	if err != nil {
		return nil
	}
	return b
}
