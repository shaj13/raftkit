package net

import "github.com/shaj13/raftkit/internal/net"

const (
	GRPC Proto = Proto(net.GRPC)
)

// Proto is a portmanteau of protocol
// and represents the underlying RPC protocol.
type Proto uint
