package disk

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash"
	"hash/crc64"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/shaj13/raftkit/api"
	"go.etcd.io/etcd/pkg/v3/fileutil"
	"go.etcd.io/etcd/pkg/v3/pbutil"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/wal/walpb"
)

const delim = '\r'

var (
	crcPool = sync.Pool{
		New: func() interface{} {
			return crc64.New(
				crc64.MakeTable(crc64.ECMA),
			)
		},
	}
	writerPool = sync.Pool{
		New: func() interface{} { return bufio.NewWriter(nil) },
	}
	readerPool = sync.Pool{
		New: func() interface{} { return bufio.NewReader(nil) },
	}
)

var (
	ErrEmptySnapshot  = errors.New("raft/disk: empty snapshot file")
	ErrSnapshotFormat = errors.New("raft/disk: invalid snapshot file format")
	ErrCRCMismatch    = errors.New("raft/disk: snapshot file corrupted, crc mismatch")
	ErrNoSnapshot     = errors.New("raft/disk: no available snapshot")
)

type SnapshotFile struct {
	Snap *raftpb.Snapshot
	Pool *api.Pool
	Data io.ReadCloser
}

func ReadNewestAvailableSnapshot(dir string, snaps []walpb.Snapshot) (*SnapshotFile, error) {
	files := map[string]struct{}{}
	target := ""
	ls, err := list(dir, snapExt)
	if err != nil {
		return nil, err
	}

	for _, name := range ls {
		files[name] = struct{}{}
	}

	for i := len(snaps) - 1; i >= 0; i-- {
		name := fmt.Sprintf(format, snaps[i].Term, snaps[i].Index) + snapExt
		if _, ok := files[name]; ok {
			target = name
			break
		}
	}

	if len(target) == 0 {
		return nil, ErrNoSnapshot
	}

	return ReadSnapshot(filepath.Join(dir, target))
}

func ReadSnapshot(path string) (sf *SnapshotFile, err error) {
	sf = new(SnapshotFile)
	sf.Snap = new(raftpb.Snapshot)
	sf.Pool = new(api.Pool)
	sf.Data, err = readSnapshotByblocks(path, sf.Snap, sf.Pool)
	return
}

func PeekSnapshot(path string) (*raftpb.Snapshot, error) {
	sf := new(SnapshotFile)
	sf.Snap = new(raftpb.Snapshot)
	r, err := readSnapshotByblocks(path, sf.Snap)
	if err != nil {
		return nil, err
	}
	r.Close()
	return sf.Snap, nil
}

func WriteSnapshot(path string, s *SnapshotFile) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	// header writer used to skip crc.
	hw := writerPool.Get().(*bufio.Writer)
	w := writerPool.Get().(*bufio.Writer)
	crc := crcPool.Get().(hash.Hash64)

	crc.Reset()
	hw.Reset(f)
	w.Reset(
		io.MultiWriter(crc, f),
	)

	flushAndSync := func(w *bufio.Writer) {
		w.Flush()
		fileutil.Fsync(f)
	}

	defer func() {
		flushAndSync(w)
		f.Close()

		crc.Reset()
		hw.Reset(nil)
		w.Reset(nil)

		crcPool.Put(crc)
		writerPool.Put(hw)
		writerPool.Put(w)

		if err != nil {
			os.Remove(path)
		}
	}()

	blocks := []proto.Message{
		// reserve header to rewrite it at the end.
		&api.SnapshotHeader{
			CRC: make([]byte, crc64.Size),
		},
		s.Snap,
		s.Pool,
	}

	for i, b := range blocks {
		w := w
		if i == 0 {
			w = hw
		}

		data, err := proto.Marshal(b)
		if err != nil {
			return err
		}

		if _, err := w.Write(data); err != nil {
			return err
		}

		if err := w.WriteByte(delim); err != nil {
			return err
		}

		flushAndSync(w)
	}

	_, err = io.Copy(w, s.Data)
	if err != nil {
		return err
	}

	flushAndSync(w)

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	h := &api.SnapshotHeader{
		CRC: crc.Sum(nil),
	}

	block := pbutil.MustMarshal(h)
	_, err = w.Write(block)

	return err
}

func readSnapshotByblocks(path string, msgs ...proto.Message) (rc io.ReadCloser, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	crc := crcPool.Get().(hash.Hash64)
	r := snapshotFileReader{
		f:      f,
		Reader: readerPool.Get().(*bufio.Reader),
	}
	r.Reset(f)
	crc.Reset()

	defer func() {
		crc.Reset()
		crcPool.Put(crc)

		if err != nil {
			r.Close()
		}
	}()

	n := 0
	header := new(api.SnapshotHeader)
	msgs = append(msgs, nil)
	copy(msgs[1:], msgs[:])
	msgs[0] = header

	_, err = r.Peek(1)
	if err != nil {
		return nil, ErrEmptySnapshot
	}

	for i, m := range msgs {
		block, err := r.ReadBytes(delim)
		if err == io.EOF {
			return nil, ErrSnapshotFormat
		}

		if err != nil {
			return nil, err
		}

		if i != 0 {
			crc.Write(block)
		}

		if len(block) > 0 {
			// remove delim.
			block = block[:len(block)-1]
		}

		if err := proto.Unmarshal(block, m); err != nil {
			return nil, err
		}

		n += len(block) + 1
	}

	_, err = io.Copy(crc, r)
	if err != nil {
		return nil, err
	}

	if bytes.Compare(crc.Sum(nil), header.CRC) != 0 {
		return nil, ErrCRCMismatch
	}

	f.Seek(int64(n), 0)
	r.Reset(f)

	return r, nil
}

type snapshotFileReader struct {
	*bufio.Reader
	f *os.File
}

func (s snapshotFileReader) Close() error {
	s.Reader.Reset(nil)
	readerPool.Put(s.Reader)
	s.Reader = nil
	return s.f.Close()
}
