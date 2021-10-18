package rpc

import "github.com/shaj13/raftkit/internal/rpc"

const (
	GRPC Proto = Proto(rpc.GRPC)
	HTTP Proto = Proto(rpc.HTTP)
)

// Proto is a portmanteau of protocol
// and represents the underlying RPC protocol.
type Proto uint

// Server represents an RPC Server.
type Server = rpc.Server
