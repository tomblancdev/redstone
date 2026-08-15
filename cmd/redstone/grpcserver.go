package main

import (
	"context"
	"encoding/json"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	corev1 "github.com/tomblancdev/redstone/gen/redstone/core/v1"
)

// The gRPC transport — services. Same selection logic as HTTP; refusals are
// FAILED_PRECONDITION with the refusal JSON in details (the 409 body's twin).
type registerServer struct {
	corev1.UnimplementedRegisterServiceServer
	k *Kernel
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *registerServer) Bind(ctx context.Context, req *corev1.BindRequest) (*corev1.BindResponse, error) {
	ok, refusal, bad := s.k.SelectInstance(BindParams{
		Capability: req.Capability,
		Level:      req.Level,
		Name:       req.Name,
		Labels:     req.Labels,
		Consumer:   req.Consumer,
		As:         req.As,
		Stack:      req.Stack,
	})
	if bad != "" {
		return nil, status.Error(codes.InvalidArgument, bad)
	}
	if refusal != nil {
		body, _ := json.Marshal(refusal)
		return nil, status.Error(codes.FailedPrecondition, string(body))
	}
	flags, err := structpb.NewStruct(ok.Flags)
	if err != nil {
		flags, _ = structpb.NewStruct(map[string]any{})
	}
	return &corev1.BindResponse{
		Name:           ok.Name,
		Capability:     ok.Capability,
		RequestedLevel: ok.RequestedLevel,
		EffectiveLevel: deref(ok.EffectiveLevel),
		Verified:       ok.Verified,
		Endpoint:       ok.Endpoint,
		Public:         ok.Public,
		Implementation: deref(ok.Implementation),
		Flags:          flags,
	}, nil
}

func (s *registerServer) ListInstances(ctx context.Context, _ *corev1.ListInstancesRequest) (*corev1.ListInstancesResponse, error) {
	var out []*corev1.Instance
	for _, inst := range s.k.Catalog() {
		out = append(out, &corev1.Instance{
			Name:           inst.Name,
			Capability:     inst.Capability,
			Level:          inst.Level,
			Endpoint:       inst.Endpoint,
			Public:         inst.Public,
			Labels:         inst.Labels,
			Verified:       inst.Verified,
			EffectiveLevel: deref(inst.EffectiveLevel),
		})
	}
	return &corev1.ListInstancesResponse{Instances: out}, nil
}

func (s *registerServer) GetGraph(ctx context.Context, _ *corev1.GetGraphRequest) (*corev1.GetGraphResponse, error) {
	var out []*corev1.BindEdge
	for _, e := range s.k.GraphEdges() {
		out = append(out, &corev1.BindEdge{
			Stack:      e.Stack,
			Consumer:   e.Consumer,
			As:         e.As,
			Capability: e.Capability,
			Level:      e.Level,
			Instance:   e.Instance,
			Verified:   e.Verified,
			At:         e.At,
		})
	}
	return &corev1.GetGraphResponse{Edges: out}, nil
}

func serveGRPC(k *Kernel, addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	server := grpc.NewServer()
	corev1.RegisterRegisterServiceServer(server, &registerServer{k: k})
	log.Printf("⚡ redstone grpc on %s", addr)
	if err := server.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
