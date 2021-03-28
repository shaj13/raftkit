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
	"github.com/shaj13/raftkit/internal/storage"
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
		New: func() interface{} {
			w := bufio.NewWriter(nil)
			return &snapshotFileWriter{Writer: w}
		},
	}

	readerPool = sync.Pool{
		New: func() interface{} {
			r := bufio.NewReader(nil)
			return &snapshotFileReader{Reader: r}
		},
	}
)

var (
	ErrEmptySnapshot  = errors.New("raft/disk: empty snapshot file")
	ErrSnapshotFormat = errors.New("raft/disk: invalid snapshot file format")
	ErrCRCMismatch    = errors.New("raft/disk: snapshot file corrupted, crc mismatch")
	ErrClosedSnapshot = errors.New("raft/disk: read/write on closed snapshot")
	ErrNoSnapshot     = errors.New("raft/disk: no available snapshot")
)

func snapshotName(term, index uint64) string {
	return fmt.Sprintf(format, term, index) + snapExt
}

func decodeNewestAvailableSnapshot(dir string, snaps []walpb.Snapshot) (*storage.SnapshotFile, error) {
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
		name := snapshotName(snaps[i].Term, snaps[i].Index)
		if _, ok := files[name]; ok {
			target = name
			break
		}
	}

	if len(target) == 0 {
		return nil, ErrNoSnapshot
	}

	return decodeSnapshot(filepath.Join(dir, target))
}

func decodeSnapshot(path string) (sf *storage.SnapshotFile, err error) {
	sf = new(storage.SnapshotFile)
	sf.Snap = new(raftpb.Snapshot)
	sf.Pool = new(api.Pool)
	sf.Data, err = decodeSnapshotByblocks(path, sf.Snap, sf.Pool)
	return
}

func peekSnapshot(path string) (*raftpb.Snapshot, error) {
	sf := new(storage.SnapshotFile)
	sf.Snap = new(raftpb.Snapshot)
	r, err := decodeSnapshotByblocks(path, sf.Snap)
	if err != nil {
		return nil, err
	}
	r.Close()
	return sf.Snap, nil
}

func encodeSnapshot(path string, s *storage.SnapshotFile) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	// header writer used to skip crc.
	hw := writerPool.Get().(*snapshotFileWriter)
	w := writerPool.Get().(*snapshotFileWriter)
	crc := crcPool.Get().(hash.Hash64)

	crc.Reset()
	hw.Reset(f, nil)
	w.Reset(
		f,
		io.MultiWriter(crc, f),
	)

	flushAndSync := func(w *snapshotFileWriter) {
		w.Flush()
		fileutil.Fsync(f)
	}

	defer func() {
		flushAndSync(w)

		hw.Close()
		w.Close()

		crcPool.Put(crc)

		if err != nil {
			os.Remove(path)
		}
	}()

	msgs := []proto.Message{
		// reserve header to rewrite it at the end.
		&api.SnapshotHeader{
			CRC: make([]byte, crc64.Size),
		},
		s.Snap,
		s.Pool,
	}

	for i, m := range msgs {
		w := w
		if i == 0 {
			w = hw
		}

		block, err := proto.Marshal(m)
		if err != nil {
			return err
		}

		if _, err := w.Write(block); err != nil {
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

func decodeSnapshotByblocks(path string, msgs ...proto.Message) (rc io.ReadCloser, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	crc := crcPool.Get().(hash.Hash64)
	r := readerPool.Get().(*snapshotFileReader)
	r.Reset(f)
	crc.Reset()

	defer func() {
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
	f   *os.File
	err error
}

func (s *snapshotFileReader) Read(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.Reader.Read(p)
}

func (s *snapshotFileReader) Close() error {
	if s.err != nil {
		return s.err
	}
	f := s.f
	s.err = ErrClosedSnapshot
	s.f = nil
	s.Reader.Reset(nil)
	readerPool.Put(s)
	return f.Close()
}

func (s *snapshotFileReader) Reset(f *os.File) {
	s.f = f
	s.err = nil
	s.Reader.Reset(f)
}

type snapshotFileWriter struct {
	*bufio.Writer
	f   *os.File
	err error
}

func (s *snapshotFileWriter) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}

	return s.Writer.Write(p)
}

func (s *snapshotFileWriter) Reset(f *os.File, w io.Writer) {
	var writer io.Writer = f
	s.f = f
	s.err = nil
	if w != nil {
		writer = w
	}
	s.Writer.Reset(writer)
}

func (s *snapshotFileWriter) Close() error {
	if s.err != nil {
		return s.err
	}
	f := s.f
	s.err = ErrClosedSnapshot
	s.f = nil
	s.Writer.Reset(nil)
	writerPool.Put(s)
	return f.Close()
}
