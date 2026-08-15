package sdk

import (
	"testing"

	corev1 "github.com/tomblancdev/redstone/gen/redstone/core/v1"
)

func TestOptsBuildRequest(t *testing.T) {
	req := &corev1.BindRequest{Stack: "s", Consumer: "a", As: "t"}
	for _, opt := range []Opt{
		WithCapability("blob", "mutable"),
		WithName("minio-local"),
		WithLabels(map[string]string{"cost": "low"}),
	} {
		opt(req)
	}
	if req.Capability != "blob" || req.Level != "mutable" || req.Name != "minio-local" || req.Labels["cost"] != "low" {
		t.Fatalf("opts misapplied: %+v", req)
	}
}

func TestEdgeHeader(t *testing.T) {
	b := &Binding{edge: "prod/admin/blob"}
	if k, v := b.Header(); k != "X-Edge" || v != "prod/admin/blob" {
		t.Fatalf("header %s=%s", k, v)
	}
}

func TestDialIsLazy(t *testing.T) {
	// No listener on this address — Dial must still succeed (lazy connect).
	c, err := Dial("127.0.0.1:1", "s", "a")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
}
