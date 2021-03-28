package disk

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/shaj13/raftkit/internal/storage"
	"go.etcd.io/etcd/raft/v3"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

var _ storage.Snapshoter = &snapshoter{}

type snapshoter struct {
	snapdir string
}

func (s snapshoter) Reader(_ context.Context, m raftpb.Message) (string, io.ReadCloser, error) {
	if raft.IsEmptySnap(m.Snapshot) {
		return "", nil, ErrEmptySnapshot
	}

	name := snapshotName(m.Snapshot.Metadata.Term, m.Snapshot.Metadata.Index)
	path := filepath.Join(s.snapdir, name)
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}

	r := readerPool.Get().(*snapshotFileReader)
	r.Reset(f)

	return name, r, nil
}

func (s snapshoter) Writer(_ context.Context, name string) (io.WriteCloser, func() (raftpb.Snapshot, error), error) {
	path := filepath.Join(s.snapdir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}

	w := writerPool.Get().(*snapshotFileWriter)
	w.Reset(f, nil)

	peek := func() (raftpb.Snapshot, error) {
		s, err := peekSnapshot(name)
		if err != nil {
			_ = os.Remove(path)
			return raftpb.Snapshot{}, err
		}
		return *s, nil
	}

	return w, peek, nil
}
