package raft

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/shaj13/raft/internal/membership"
	"github.com/shaj13/raft/internal/raftengine"
	"github.com/shaj13/raft/internal/raftpb"
	"github.com/shaj13/raft/internal/storage"
	"github.com/shaj13/raft/internal/transport"
	etcdraftpb "go.etcd.io/etcd/raft/v3/raftpb"
)

type controller struct {
	node    *Node
	engine  raftengine.Engine
	pool    membership.Pool
	storage storage.Storage
}

func (c *controller) Join(ctx context.Context, gid uint64, m *raftpb.Member) (*raftpb.JoinResponse, error) {
	var err error

	if _, ok := c.node.GetMemebr(m.ID); !ok {
		err = c.node.AddMember(ctx, m)
	} else {
		err = c.node.UpdateMember(ctx, m)
	}

	if err != nil {
		return nil, err
	}

	resp := &raftpb.JoinResponse{
		ID:      m.ID,
		Members: c.pool.Snapshot(),
	}

	return resp, nil
}

func (c *controller) Push(ctx context.Context, gid uint64, m etcdraftpb.Message) error {
	return c.engine.Push(m)
}

func (c *controller) PromoteMember(ctx context.Context, gid uint64, m raftpb.Member) error {
	return c.node.promoteMember(ctx, m.ID, true)
}

func (c *controller) SnapshotWriter(gid, term, index uint64) (io.WriteCloser, error) {
	return c.storage.Snapshotter().Writer(term, index)
}

func (c *controller) SnapshotReader(gid, term uint64, index uint64) (io.ReadCloser, error) {
	return c.storage.Snapshotter().Reader(term, index)
}

type router struct {
	mu    sync.Mutex
	ctrls map[uint64]transport.Controller
}

func (r *router) add(gid uint64, ctrl transport.Controller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctrls[gid] = ctrl
}

func (r *router) remove(gid uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ctrls, gid)
}

func (r *router) get(gid uint64) (transport.Controller, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.ctrls[gid]
	if !ok {
		return nil, fmt.Errorf("raft: unknown group id %d", gid)
	}
	return ctrl, nil
}

func (r *router) Join(ctx context.Context, gid uint64, m *raftpb.Member) (*raftpb.JoinResponse, error) {
	ctrl, err := r.get(gid)
	if err != nil {
		return nil, err
	}
	return ctrl.Join(ctx, gid, m)
}

func (r *router) Push(ctx context.Context, gid uint64, m etcdraftpb.Message) error {
	ctrl, err := r.get(gid)
	if err != nil {
		return err
	}
	return ctrl.Push(ctx, gid, m)
}

func (r *router) PromoteMember(ctx context.Context, gid uint64, m raftpb.Member) error {
	ctrl, err := r.get(gid)
	if err != nil {
		return err
	}
	return ctrl.PromoteMember(ctx, gid, m)
}

func (r *router) SnapshotWriter(gid, term, index uint64) (io.WriteCloser, error) {
	ctrl, err := r.get(gid)
	if err != nil {
		return nil, err
	}

	return ctrl.SnapshotWriter(gid, term, index)
}

func (r *router) SnapshotReader(gid, term, index uint64) (io.ReadCloser, error) {
	ctrl, err := r.get(gid)
	if err != nil {
		return nil, err
	}
	return ctrl.SnapshotReader(gid, term, index)
}
