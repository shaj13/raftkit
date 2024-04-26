package membership

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/shaj13/raft/internal/raftpb"
	"github.com/shaj13/raft/internal/transport"
	"github.com/shaj13/raft/raftlog"
	"github.com/stretchr/testify/require"
)

func TestPoolNewMember(t *testing.T) {
	m := raftpb.Member{
		Address: ":5052",
		ID:      123,
		Type:    raftpb.LocalMember,
	}

	p := New(testConfig(t)).(*pool)

	mem, err := p.newMember(m)
	require.NoError(t, err)
	require.Equal(t, m.Address, mem.Address())
	require.Equal(t, m.ID, mem.ID())
	require.Equal(t, m.Type, mem.Type())
}

func TestPoolNextID(t *testing.T) {
	p := New(testConfig(t)).(*pool)
	rec := make(map[uint64]struct{})

	for i := 0; i < 10; i++ {
		id := p.NextID()
		_, ok := rec[id]
		if ok {
			t.Error("pool generate same id more than once")
		}
		p.membs[id] = nil
		rec[id] = struct{}{}
	}
}

func TestPoolUpdate(t *testing.T) {
	m := &raftpb.Member{ID: 1, Type: raftpb.LocalMember}
	p := New(testConfig(t))
	p.Add(*m)
	m.Address = "5050"
	p.Update(*m)
	mem, _ := p.Get(m.ID)
	require.Equal(t, m.Address, mem.Address())
}

func TestPoolRemoveErr(t *testing.T) {
	p := New(testConfig(t))
	err := p.Remove(raftpb.Member{})
	require.Contains(t, err.Error(), "not found")
}

func TestPoolRestore(t *testing.T) {
	p := New(testConfig(t))
	ids := make(map[uint64]struct{})
	membs := make([]raftpb.Member, 5)

	for i := 0; i < 5; i++ {
		m := raftpb.Member{ID: uint64(i), Type: raftpb.LocalMember}
		ids[uint64(i)] = struct{}{}
		membs[i] = m
	}

	p.Restore(membs)

	for id := range ids {
		_, ok := p.Get(id)
		if !ok {
			t.Errorf("pool have'nt add member %d during the restore", id)
		}
	}
}

func TestPoolSnapshot(t *testing.T) {
	p := New(testConfig(t))
	p.Add(raftpb.Member{ID: p.NextID(), Type: raftpb.LocalMember})
	membs := p.Snapshot()
	require.Equal(t, len(p.Members()), len(membs))
}

func TestPoolRemove(t *testing.T) {
	m := &raftpb.Member{ID: 1, Type: raftpb.LocalMember}
	ctrl := gomock.NewController(t)
	cfg := NewMockConfig(ctrl)
	r := NewMockReporter(ctrl)
	r.EXPECT().ReportShutdown(gomock.Eq(m.ID)).Return()
	cfg.EXPECT().Reporter().Return(r).MaxTimes(2)
	cfg.EXPECT().Logger().Return(raftlog.DefaultLogger)

	p := New(cfg)
	p.Add(*m)

	// run twice to check removing
	// a removed member does not return error
	for i := 0; i < 2; i++ {
		m.Type = raftpb.RemovedMember
		err := p.Remove(*m)
		mem, _ := p.Get(m.ID)
		require.NoError(t, err)
		require.Equal(t, raftpb.RemovedMember, mem.Type())
	}
}

func TestPoolClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	mem := NewMockMember(ctrl)
	mem.EXPECT().TearDown(gomock.Any()).Return(errRemovedMember)
	p := new(pool)
	p.membs = map[uint64]Member{
		1: mem,
	}
	err := p.TearDown(context.TODO())
	require.Equal(t, errRemovedMember, err)
	require.Equal(t, len(p.membs), 0)
}

func testConfig(t *testing.T) *MockConfig {
	ctrl := gomock.NewController(t)
	cfg := NewMockConfig(ctrl)
	cfg.EXPECT().Reporter().Return(nil).AnyTimes()
	cfg.EXPECT().DrainTimeout().Return(time.Duration(-1)).AnyTimes()
	cfg.EXPECT().StreamTimeout().Return(time.Duration(-1)).AnyTimes()
	cfg.EXPECT().Context().Return(context.TODO()).AnyTimes()
	cfg.EXPECT().Logger().Return(raftlog.DefaultLogger).AnyTimes()
	return cfg
}

func mockDial(c transport.Client, err error) transport.Dial {
	return func(ctx context.Context, addr string) (transport.Client, error) {
		return c, err
	}
}
