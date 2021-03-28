package storage

import (
	"context"
	"io"

	"github.com/shaj13/raftkit/api"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

type SnapshotFile struct {
	Snap *raftpb.Snapshot
	Pool *api.Pool
	Data io.ReadCloser
}

type Snapshoter interface {
	Reader(context.Context, raftpb.Message) (string, io.ReadCloser, error)
	Writer(context.Context, string) (io.WriteCloser, func() (raftpb.Snapshot, error), error)
}
