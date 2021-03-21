package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/shaj13/raftkit/api"
	raft "github.com/shaj13/raftkit/api"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

const maxh = 31

func main() {
	s := snapshoter{"/tmp"}
	snap := raftpb.Snapshot{
		Metadata: raftpb.SnapshotMetadata{
			Index: 130,
			Term:  320,
		},
	}
	pool := api.Member{
		ID:      121,
		Address: ":50052",
		Type:    api.SelfMember,
	}

	r := strings.NewReader("my app data")

	err := s.save(snap, pool, r)
	fmt.Println(err)

	rsnap, rpool, rr, err := s.load()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("snap := ", rsnap.Metadata.Term, rsnap.Metadata.Index)
	fmt.Println("pool := ", rpool.ID, rpool.Address, rpool.Type)
	buf, _ := ioutil.ReadAll(rr)
	fmt.Println("app data := ", string(buf))
}

type snapshoter struct {
	dir string
}

func (s snapshoter) save(snap raftpb.Snapshot, pool api.Member, r io.Reader) error {
	h := &raft.SnapHeader{}
	// filename := fmt.Sprintf("%016x-%016x.snap", snap.Metadata.Index, snap.Metadata.Term)
	filename := fmt.Sprintf("%016x-%016x.snap", 0, 0)
	path := filepath.Join(s.dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	sbuf, err := snap.Marshal()
	if err != nil {
		return err
	}

	pbuf, err := pool.Marshal()
	if err != nil {
		return err
	}

	hbuf := make([]byte, maxh)
	h.Metadata = int64(len(sbuf))
	h.Pool = int64(len(pbuf))
	n, err := h.MarshalTo(hbuf)
	if err != nil {
		return err
	}
	hbuf[n] = '\n'
	f.Write(hbuf)
	f.Write(sbuf)
	f.Write(pbuf)
	io.Copy(f, r)
	return nil
}

func (s snapshoter) load() (*raftpb.Snapshot, *api.Member, io.Reader, error) {
	filename := fmt.Sprintf("%016x-%016x.snap", 0, 0)
	path := filepath.Join(s.dir, filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}

	h := new(api.SnapHeader)
	hbuf := make([]byte, maxh)

	if _, err := f.Read(hbuf); err != nil {
		return nil, nil, nil, err
	}

	n := bytes.LastIndexByte(hbuf, '\n')
	if n == 0 {
		return nil, nil, nil, fmt.Errorf("invalid snap format")
	}

	hbuf = hbuf[:n]

	if err := h.Unmarshal(hbuf); err != nil {
		panic(err)
	}

	pool := new(api.Member)
	snap := new(raftpb.Snapshot)
	sbuf := make([]byte, h.Metadata)
	pbuf := make([]byte, h.Pool)

	if _, err := f.Read(sbuf); err != nil {
		return nil, nil, nil, err
	}

	if _, err := f.Read(pbuf); err != nil {
		return nil, nil, nil, err
	}

	if err := pool.Unmarshal(pbuf); err != nil {
		return nil, nil, nil, err
	}

	if err := snap.Unmarshal(sbuf); err != nil {
		return nil, nil, nil, err
	}

	return snap, pool, f, nil
}
