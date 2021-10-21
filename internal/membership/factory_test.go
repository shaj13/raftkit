package membership

import (
	"context"
	"testing"

	"github.com/shaj13/raftkit/internal/raftpb"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFactory(t *testing.T) {
	m := raftpb.Member{
		Address: ":5052",
		ID:      123,
		Type:    raftpb.LocalMember,
	}

	f := newFactory(
		context.Background(),
		testConfig,
	)

	mem, ok, err := f.From(m)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, m.Address, mem.Address())
	require.Equal(t, m.ID, mem.ID())
	require.Equal(t, m.Type, mem.Type())

	mem, ok, err = f.Cast(mem, raftpb.RemovedMember)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, m.Address, mem.Address())
	require.Equal(t, m.ID, mem.ID())
	require.Equal(t, raftpb.RemovedMember, mem.Type())

	mm := f.To(mem)
	m.Type = raftpb.RemovedMember
	require.Equal(t, m, mm)
}

func TestNewRemote(t *testing.T) {
	tr := &mockRPC{mock.Mock{}}
	tr.On("Close").Return(nil)
	dial := mockDial(tr, nil)
	cfg := mockConfig{d: dial}
	m, _ := newRemote(context.Background(), cfg, 0, "")
	m.Close()
	tr.AssertCalled(t, "Close")
}
