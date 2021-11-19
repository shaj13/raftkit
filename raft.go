package raft

import (
	"github.com/shaj13/raft/internal/membership"
	"github.com/shaj13/raft/internal/raftengine"
	"github.com/shaj13/raft/internal/storage/disk"
	itransport "github.com/shaj13/raft/internal/transport"
	"github.com/shaj13/raft/transport"
)

func New(fsm StateMachine, proto transport.Proto, opts ...Option) *Node {
	if fsm == nil {
		panic("raft: cannot create node from nil state machine")
	}

	cfg := newConfig(opts...)
	cfg.fsm = fsm

	newHandler, dialer := itransport.Proto(proto).Get()
	cfg.controller = new(controller)
	cfg.storage = disk.New(cfg)
	cfg.dial = dialer(cfg)
	cfg.pool = membership.New(cfg)
	cfg.engine = raftengine.New(cfg)

	node := new(Node)
	node.pool = cfg.pool
	node.engine = cfg.engine
	node.storage = cfg.storage
	node.dial = cfg.dial
	node.cfg = cfg
	node.handler = newHandler(cfg)

	cfg.controller.(*controller).node = node
	cfg.controller.(*controller).engine = cfg.engine
	cfg.controller.(*controller).pool = cfg.pool
	cfg.controller.(*controller).storage = cfg.storage

	return node
}
