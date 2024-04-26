package membership

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/shaj13/raft/internal/raftpb"
	"github.com/shaj13/raft/raftlog"
	"golang.org/x/sync/errgroup"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// New construct and returns a new pool members.
func New(cfg Config) Pool {
	return &pool{
		cfg:    cfg,
		logger: cfg.Logger(),
		membs:  make(map[uint64]Member),
		matcher: func(m raftpb.Member) raftpb.MemberType {
			return m.Type
		},
	}
}

type pool struct {
	cfg     Config
	logger  raftlog.Logger
	matcher func(m raftpb.Member) raftpb.MemberType
	mu      sync.Mutex // protects the membs
	membs   map[uint64]Member
}

func (p *pool) RegisterTypeMatcher(fn func(m raftpb.Member) raftpb.MemberType) {
	p.matcher = fn
}

func (p *pool) NextID() uint64 {
	var id uint64
	for {
		id = uint64(rand.Int63()) + 1
		if _, ok := p.Get(id); !ok {
			break
		}
	}
	return id
}

func (p *pool) Members() []Member {
	membs := []Member{}
	p.mu.Lock()
	for _, m := range p.membs {
		membs = append(membs, m)
	}
	p.mu.Unlock()
	return membs
}

func (p *pool) Get(id uint64) (Member, bool) {
	p.mu.Lock()
	m, ok := p.membs[id]
	p.mu.Unlock()
	return m, ok
}

func (p *pool) Add(m raftpb.Member) error {
	_, ok := p.Get(m.ID)
	if ok {
		return p.Update(m)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	mem, err := p.newMember(m)
	if err != nil {
		return err
	}

	p.membs[m.ID] = mem
	return nil
}

func (p *pool) Update(m raftpb.Member) error {
	mem, ok := p.Get(m.ID)
	if !ok {
		return p.Add(m)
	}

	return mem.Update(m)
}

func (p *pool) Remove(m raftpb.Member) error {
	mem, ok := p.Get(m.ID)
	if !ok {
		return fmt.Errorf("raft/membership: member %d not found", m.ID)
	}

	if mem.Type() == m.Type {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := mem.Close(); err != nil {
		p.logger.Warningf("raft.membership: closing member %d: %v", m.ID, err)
	}

	mem, err := p.newMember(m)
	if err != nil {
		return err
	}

	p.membs[m.ID] = mem
	return nil
}

func (p *pool) Snapshot() []raftpb.Member {
	p.mu.Lock()
	defer p.mu.Unlock()
	membs := []raftpb.Member{}
	for _, mem := range p.membs {
		membs = append(membs, mem.Raw())
	}
	return membs
}

func (p *pool) Restore(membs []raftpb.Member) {
	for _, m := range membs {
		if err := p.Add(m); err != nil {
			p.logger.Errorf("raft.membership: adding member %d: %v", m.ID, err)
		}
	}
}

func (p *pool) TearDown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	eg := new(errgroup.Group)
	for _, mem := range p.membs {
		// The following wrapper needed to sync the mem with the goroutine,
		// Otherwise, all goroutines will use the last mem from the range.
		fn := func(mem Member) func() error {
			return func() error {
				return mem.TearDown(ctx)
			}
		}
		eg.Go(fn(mem))
	}
	p.membs = make(map[uint64]Member)
	return eg.Wait()
}

func (p *pool) newMember(m raftpb.Member) (Member, error) {
	switch p.matcher(m) {
	case raftpb.VoterMember, raftpb.LearnerMember, raftpb.StagingMember:
		return newRemote(p.cfg, m)
	case raftpb.RemovedMember:
		return newRemoved(p.cfg, m)
	case raftpb.LocalMember:
		return newLocal(p.cfg, m)
	default:
		return nil, fmt.Errorf("raft/membership: unknown member type %s", m.Type)
	}
}
