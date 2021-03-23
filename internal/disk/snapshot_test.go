package disk

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shaj13/raftkit/api"
	"github.com/stretchr/testify/assert"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/wal/walpb"
)

func TestSnapshotReadWrite(t *testing.T) {
	dir := createTestDir("read-write", t)
	path := filepath.Join(dir, t.Name())
	defer os.RemoveAll(dir)

	expected, expectedData := snapshotTestFile()
	err := WriteSnapshot(path, &expected)
	assert.NoError(t, err)

	got, err := ReadSnapshot(path)
	assert.NoError(t, err)
	assert.Equal(t, expected.Snap, got.Snap)
	assert.Equal(t, expected.Pool, got.Pool)

	gotData, err := ioutil.ReadAll(got.Data)
	assert.NoError(t, err)
	assert.Equal(t, expectedData, string(gotData))
}

func TestPeekSnapshot(t *testing.T) {
	expected, _ := snapshotTestFile()

	// Round #1 it return error when file invalid
	snap, err := PeekSnapshot("")
	assert.Error(t, err)
	assert.Nil(t, snap)

	// Round #2 it return snap object
	snap, err = PeekSnapshot("./testdata/valid.snap")
	assert.NoError(t, err)
	assert.Equal(t, expected.Snap, snap)
}

func TestSnapErr(t *testing.T) {
	table := []struct {
		name     string
		file     string
		contains string
	}{
		{
			name:     "it return error when file does not exist",
			file:     "test",
			contains: "no such file or directory",
		},
		{
			name:     "it return error when file have unexpected EOF",
			file:     "./testdata/ueof.snap",
			contains: io.ErrUnexpectedEOF.Error(),
		},
		{
			name:     "it return error when snapshot empty",
			file:     "./testdata/empty.snap",
			contains: ErrEmptySnapshot.Error(),
		},
		{
			name:     "it return error when snapshot have invalid format",
			file:     "./testdata/format.snap",
			contains: ErrSnapshotFormat.Error(),
		},
		{
			name:     "it return error when snapshot have invalid crc",
			file:     "./testdata/crc.snap",
			contains: ErrCRCMismatch.Error(),
		},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadSnapshot(tt.file)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

func TestReadNewestAvailableSnapshot(t *testing.T) {
	// Round #1 it return error when snapshots dir does not exist
	sf, err := ReadNewestAvailableSnapshot("", []walpb.Snapshot{})
	assert.Nil(t, sf)
	assert.Contains(t, err.Error(), "no such file or directory")

	// Round #2 it return error when no snapshots
	sf, err = ReadNewestAvailableSnapshot("./testdata/", []walpb.Snapshot{})
	assert.Nil(t, sf)
	assert.Equal(t, ErrNoSnapshot, err)

	// Round #3 it return latest snapshots
	expected, _ := snapshotTestFile()
	sf, err = ReadNewestAvailableSnapshot("./testdata/", []walpb.Snapshot{{Index: 3, Term: 3}})
	assert.NoError(t, err)
	assert.Equal(t, expected.Snap, sf.Snap)
}

func snapshotTestFile() (SnapshotFile, string) {
	const data = "some app data"
	return SnapshotFile{
		Snap: &raftpb.Snapshot{
			Metadata: raftpb.SnapshotMetadata{
				ConfState: raftpb.ConfState{
					Voters: []uint64{1, 2, 3},
				},
				Index: 1,
				Term:  1,
			},
		},
		Pool: &api.Pool{
			Members: []api.Member{
				{
					Address: ":50052",
					ID:      11,
				},
			},
		},
		Data: ioutil.NopCloser(strings.NewReader(data)),
	}, data
}
