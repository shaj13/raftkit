package raft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/shaj13/raft/internal/membership"
	"github.com/shaj13/raft/internal/raftengine"
	"github.com/shaj13/raft/internal/raftpb"
	"github.com/shaj13/raft/internal/storage"
	"github.com/shaj13/raft/internal/storage/disk"
	"github.com/shaj13/raft/internal/transport"
	etransport "github.com/shaj13/raft/transport"
	etcdraftpb "go.etcd.io/etcd/raft/v3/raftpb"
)

var (
	// ErrNodeStopped is returned by the Node methods after a call to
	// Shutdown or when it has not started.
	ErrNodeStopped = raftengine.ErrStopped
	// ErrNotLeader is returned when an operation can't be completed on a
	// follower or candidate node
	ErrNotLeader = errors.New("raft: node is not the leader")
)

// NewNode construct a new node from the given configuration.
// The returned node is in a stopped state, therefore it must be start explicitly.
func NewNode(fsm StateMachine, proto etransport.Proto, opts ...Option) *Node {
	if fsm == nil {
		panic("raft: cannot create node from nil state machine")
	}

	newHandler, dialer := transport.Proto(proto).Get()
	ctrl := new(controller)
	cfg := newConfig(opts...)
	cfg.fsm = fsm
	cfg.controller = ctrl
	cfg.storage = disk.New(cfg)
	cfg.dial = dialer(cfg)
	cfg.pool = membership.New(cfg)
	cfg.engine = raftengine.New(cfg)

	node := new(Node)
	node.pool = cfg.pool
	node.engine = cfg.engine
	node.storage = cfg.storage
	node.dial = cfg.dial
	node.cfg = cfg
	node.handler = newHandler(cfg)

	ctrl.node = node
	ctrl.engine = cfg.engine
	ctrl.pool = cfg.pool
	ctrl.storage = cfg.storage

	return node
}

// NewNodeGroup returns a new NodeGroup.
// the returned node group will lazily initialize,
// from the first node registered within it, So it's recommended to apply
// the same  HeartbeatTick, ElectionTick, and TickInterval configuration to all sub-nodes.
func NewNodeGroup(proto etransport.Proto) *NodeGroup {
	cfg := newConfig()
	nh, _ := transport.Proto(proto).Get()
	mux := raftengine.NewMux()

	router := &router{
		ctrls: make(map[uint64]transport.Controller),
	}
	cfg.controller = router

	return &NodeGroup{
		proto:   proto,
		mux:     mux,
		handler: nh(cfg),
		router:  router,
	}
}

// NodeGroup manage multi Raft nodes from many different Raft groups known as Raft clusters.
// NodeGroup is more efficient than a collection of nodes.
//
// Scales raft into multiple raft groups requires data sharding,
// each raft group responsible for managing data in the range [start, end].
// as the system grows to include more ranges, so does the amount of traffic
// required to handle heartbeats. The number of ranges is much larger than the
// number of physical nodes so many ranges will have overlapping membership
// this is where NodeGroup comes in: instead of allowing each range to run Raft independently,
// we manage an entire node’s worth of ranges as a group. Each pair of physical nodes
// only needs to exchange heartbeats once per tick (coalesced heartbeats),
// no matter how many ranges they have in common.
//
// Create, Remove can run while node group stopped.
// starting an created node is required a started node group,
// Otherwise, it will hang until the node group started.
type NodeGroup struct {
	proto   etransport.Proto
	mux     raftengine.Mux
	handler transport.Handler
	router  *router
}

// Handler return NodeGroup transportation handler,
// that delegated to respond to RPC requests over the wire.
// the returned handler must be registered with the transportation server.
func (ng *NodeGroup) Handler() etransport.Handler {
	return ng.handler
}

// Start starts the NodeGroup. It can be called after Stop to restart the
// NodeGroup.
// Start returns when Stop called.
func (ng *NodeGroup) Start() {
	ng.mux.Start()
}

// Create construct and returns a new node that associated to the given group id,
//
// The node and the group are correlated so each group id must have
// its own node object and each node object must have its own group id.
//
// All registered nodes within the node group must have the same id,
// that is how multiple nodes object representing one single physical node
// that participate in multiple raft groups. Starting a node with a
// different id from the previous one will cause a panic.
// Make sure the program set the node id using option, if it's not first node.
func (ng *NodeGroup) Create(groupID uint64, fsm StateMachine, opts ...Option) *Node {
	n := NewNode(fsm, ng.proto, opts...)
	ng.router.add(groupID, n.cfg.controller)
	n.cfg.groupID = groupID
	n.cfg.mux = ng.mux
	n.cfg.controller = ng.router
	n.handler = ng.handler
	return n
}

// Remove remove node related to the given group id.
// after the removal, the actual node will become idle,
// it must coordinate with node shutdown explicitly.
//
//	nodeGroup.Remove(12)
//	node.Shutdown(ctx)
func (ng *NodeGroup) Remove(groupID uint64) {
	ng.router.remove(groupID)
}

// Stop performs any necessary termination of the NodeGroup.
func (ng *NodeGroup) Stop() {
	ng.mux.Stop()
}

// Node is a controller of the current effective raft member,
// It also represents the front API to proposes changes into the raft cluster.
//
// Node packed with a built-in segmented WAL to provide durability and ensure data integrity.
// alongside snapshotter that take a snapshot of the state of a system at a particular point in time.
// Although, the application must have its own backend DB delegated by the state machine interface.
//
// Node also maintains a membership pool containing all other raft members.
//
// Multiple goroutines may invoke methods on a Node simultaneously.
type Node struct {
	handler transport.Handler
	dial    transport.Dial
	pool    membership.Pool
	storage storage.Storage
	engine  raftengine.Engine
	cfg     *config
	// exec pre conditions, its used by tests.
	exec func(fns ...func(c *Node) error) error
}

// Shutdown gracefully shuts down the node without interrupting any
// active requests. Shutdown works by first closing all open
// requests listeners, then blocks until all the pending requests
// are finished, and then shut down.
// If the provided context expires before the shutdown is complete,
// Shutdown force the node to shut off, Shutdown returns any
// error returned from closing the Node's underlying internal(s).
//
// When Shutdown is called, Start may immediately return ErrNodeStopped.
// Make sure the program doesn't exit and waits instead for Shutdown to return.
func (n *Node) Shutdown(ctx context.Context) error {
	return n.engine.Shutdown(ctx)
}

// Handler return node transportation handler,
// that delegated to respond to RPC requests over the wire.
// the returned handler must be registered with the transportation server,
// unless the node is registered with a node group.
func (n *Node) Handler() etransport.Handler {
	return n.handler
}

// LinearizableRead implies that once a write completes,
// all later reads should return the value of that write,
// or the value of a later write.
func (n *Node) LinearizableRead(ctx context.Context) error {
	err := n.preCond(
		joined(),
		noLeader(),
		available(),
	)

	if err != nil {
		return err
	}

	return n.engine.LinearizableRead(ctx)
}

// Snapshot is used to manually force node to take a snapshot. Returns a io.ReadCloser
// that can be used to to read snapshot file.
// the caller must invoke close method on the returned io.ReadCloser explicitly,
// Otherwise, the underlying os.File remain open.
func (n *Node) Snapshot() (io.ReadCloser, error) {
	err := n.preCond(
		joined(),
	)

	if err != nil {
		return nil, err
	}

	snap, err := n.engine.CreateSnapshot()
	if err != nil {
		return nil, err
	}

	meta := snap.Metadata
	return n.storage.Snapshotter().Reader(meta.Term, meta.Index)
}

// TransferLeadership proposes to transfer leadership to the given member id.
func (n *Node) TransferLeadership(ctx context.Context, id uint64) error {
	err := n.preCond(
		joined(),
		notMember(id),
		memberRemoved(id),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		disableForwarding(), // TODO: verify this.
		available(),
	)

	if err != nil {
		return err
	}

	return n.engine.TransferLeadership(ctx, id)
}

// Stepdown proposes to transfer leadership to the longest active member in the cluster.
// This must be run on the leader or it will fail.
func (n *Node) Stepdown(ctx context.Context) error {
	err := n.preCond(
		joined(),
		notLeader(),
		available(),
	)

	if err != nil {
		return err
	}

	// we can do the following, because member's active since is given from the current node clock.
	longest := time.Now().Add(math.MaxInt64)
	id := n.Whoami()
	cond := func(m Member) bool {
		since := m.ActiveSince()
		ok := m.IsActive() && m.Type() == VoterMember && since.Before(longest) && id != m.ID()
		if ok {
			longest = since
			return true
		}

		return false
	}

	// get longest active member then transfer leadership to it.
	membs := n.members(cond)
	if len(membs) == 0 {
		return errors.New("raft: failed to find longest active member")
	}

	return n.engine.TransferLeadership(ctx, membs[0].ID())
}

// Start start the node and accepts incoming requests on the handler or on local node methods.
// It can be called after Stop to restart the node.
//
// Start always returns a non-nil error. After Shutdown, the returned error is ErrNodeStopped.
func (n *Node) Start(opts ...StartOption) error {
	cfg := new(startConfig)
	cfg.apply(opts...)
	return n.engine.Start(cfg.addr, cfg.operators...)
}

// Leave proposes to remove current effective member.
// See the documentation of "RemoveMember" for more information.
func (n *Node) Leave(ctx context.Context) error {
	return n.RemoveMember(
		ctx,
		n.Whoami(),
	)
}

// Replicate proposes to replicate the given data to all raft members,
// in a highly consistent manner. If the provided context expires before,
// the replication is complete,
// Replicate returns the context's error, otherwise it returns any
// error returned due to the replication.
func (n *Node) Replicate(ctx context.Context, data []byte) error {
	err := n.preCond(
		joined(),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	return n.engine.ProposeReplicate(ctx, data)
}

// UpdateMember proposes to update the given member,
// It considered complete after reaching a majority.
// After committing the update, each member in the
// cluster updates the given member configuration on its pool.
//
// If the provided context expires before, the update is complete,
// UpdateMember returns the context's error, otherwise it returns any
// error returned due to the update.
//
// Note: the member id and type are not updatable.
func (n *Node) UpdateMember(ctx context.Context, raw *RawMember) error {
	err := n.preCond(
		joined(),
		notMember(raw.ID),
		memberRemoved(raw.ID),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		addressInUse(raw.ID, raw.Address),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	mem, _ := n.GetMemebr(raw.ID)
	raw.Type = mem.Type()

	return n.engine.ProposeConfChange(ctx, raw, etcdraftpb.ConfChangeUpdateNode)
}

// RemoveMember proposes to remove the given member from the cluster,
// It considered complete after reaching a majority.
// After committing the removal, each member in the
// cluster remove the given member from its pool.
//
// Although, the removed member configuration will remain and
// only its type will be changed to "RemovedMember".
// therefore its id is not reusable again, and its cannot rejoin the cluster again.
//
// If the provided context expires before, the removal is complete,
// RemoveMember returns the context's error, otherwise it returns any
// error returned due to the removal.
func (n *Node) RemoveMember(ctx context.Context, id uint64) error {
	err := n.preCond(
		joined(),
		notMember(id),
		memberRemoved(id),
		leader(id),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	mem, _ := n.GetMemebr(id)
	raw := mem.Raw()
	raw.Type = raftpb.RemovedMember

	return n.engine.ProposeConfChange(ctx, &raw, etcdraftpb.ConfChangeRemoveNode)
}

// AddMember proposes to add the given member to the cluster,
// It considered complete after reaching a majority.
// After committing the addition, each member in the
// cluster add the given member to its pool.
//
// Although, most applications will use the basic join.
//
// If the provided context expires before, the add is complete,
// AddMember returns the context's error, otherwise it returns any
// error returned due to the add.
//
// If the provided member id is None, AddMember will assign next available id.
func (n *Node) AddMember(ctx context.Context, raw *RawMember) error {
	err := n.preCond(
		joined(),
		idInUse(raw.ID),
		addressInUse(raw.ID, raw.Address),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	if raw.ID == None {
		raw.ID = n.pool.NextID()
	}

	cct := etcdraftpb.ConfChangeAddNode
	if raw.Type != VoterMember {
		cct = etcdraftpb.ConfChangeAddLearnerNode
	}

	return n.engine.ProposeConfChange(ctx, raw, cct)
}

// PromoteMember proposes to promote a learner member to a voting member,
// It considered complete after reaching a majority.
// After committing the promotion, each member in the
// cluster updates the given member type on its pool.
//
// If the provided context expires before, the promotion is complete,
// PromoteMember returns the context's error, otherwise it returns any
// error returned due to the promotion.
func (n *Node) PromoteMember(ctx context.Context, id uint64) error {
	return n.promoteMember(ctx, id, false)
}

// DemoteMember proposes to take away a member vote.
// It considered complete after reaching a majority.
// After committing the demotion, each member in the
// cluster updates the given member type on its pool.
//
// If the provided context expires before, the promotion is complete,
// DemoteMember returns the context's error, otherwise it returns any
// error returned due to the demotion.
func (n *Node) DemoteMember(ctx context.Context, id uint64) error {
	err := n.preCond(
		joined(),
		notMember(id),
		memberRemoved(id),
		noLeader(),
		leader(id),
		notType(n.Whoami(), VoterMember),
		notType(id, VoterMember),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	mem, _ := n.GetMemebr(id)
	raw := mem.Raw()
	(&raw).Type = LearnerMember

	return n.engine.ProposeConfChange(ctx, &raw, etcdraftpb.ConfChangeAddLearnerNode)
}

// GetMemebr returns member associated to the given id if exist,
// Otherwise, it return nil and false.
func (n *Node) GetMemebr(id uint64) (Member, bool) {
	return n.pool.Get(id)
}

// Members returns the list of raft Members in the Cluster.
func (n *Node) Members() []Member {
	return n.members(func(m Member) bool { return true })
}

// Whoami returns the id associated with current effective member.
// It return None, if the node stopped or not yet part of a raft cluster.
func (n *Node) Whoami() uint64 {
	s, _ := n.engine.Status()
	return s.ID
}

// Leader returns the id of the raft cluster leader, if there any.
// Otherwise, it return None.
func (n *Node) Leader() uint64 {
	s, _ := n.engine.Status()
	return s.Lead
}

func (n *Node) members(cond func(m Member) bool) []Member {
	mems := []Member{}
	for _, m := range n.pool.Members() {
		if cond(m) {
			mems = append(mems, m)
		}
	}
	return mems
}

func (n *Node) preCond(fns ...func(c *Node) error) error {
	if n.exec != nil {
		return n.exec(fns...)
	}

	for _, fn := range fns {
		if err := fn(n); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) promoteMember(ctx context.Context, id uint64, forwarded bool) error {
	err := n.preCond(
		joined(),
		notMember(id),
		noLeader(),
		notType(n.Whoami(), VoterMember),
		notType(id, LearnerMember),
		disableForwarding(),
		available(),
	)

	if err != nil {
		return err
	}

	rs, err := n.engine.Status()
	if err != nil {
		return err
	}

	// leader may lost during forwarding,
	// if there is no progress and promotion have been forwarded to this node.
	if rs.Progress == nil && forwarded {
		return raftengine.ErrNoLeader
	}

	mem, _ := n.GetMemebr(id)
	raw := mem.Raw()

	if rs.Progress == nil {
		lmem, ok := n.GetMemebr(rs.Lead)
		// leader lost, because rs.Lead = None.
		if !ok {
			return raftengine.ErrNoLeader
		}

		client, err := n.dial(ctx, lmem.Address())
		if err != nil {
			return err
		}

		n.cfg.logger.V(3).Infof("raft.node: forwarding member %d promotion to %d", id, lmem.ID())
		return client.PromoteMember(ctx, raw)
	}

	leader := rs.Progress[rs.Lead].Match
	learner := rs.Progress[id].Match
	// the learner's Match not caught up with the leader yet.
	if float64(learner) < float64(leader)*0.9 {
		return fmt.Errorf("raft: promotion failed, memebr %d not synced with the leader yet", id)
	}

	(&raw).Type = VoterMember

	return n.engine.ProposeConfChange(ctx, &raw, etcdraftpb.ConfChangeAddNode)
}

func joined() func(c *Node) error {
	return func(c *Node) error {
		if c.Whoami() == None {
			return fmt.Errorf("raft: node is not yet part of a raft cluster")
		}
		return nil
	}
}

func available() func(c *Node) error {
	return func(c *Node) error {
		reachables := c.members(func(m Member) bool {
			return m.IsActive() && m.Type() == VoterMember
		})

		voters := c.members(func(m Member) bool {
			return m.Type() == VoterMember
		})

		if len(reachables) < len(voters)/2+1 {
			return fmt.Errorf("raft: quorum lost and the cluster unavailable, no new logs can be committed")
		}
		return nil
	}
}

func notMember(id uint64) func(c *Node) error {
	return func(c *Node) error {
		if _, ok := c.GetMemebr(id); !ok {
			return fmt.Errorf("raft: unknown member %d", id)
		}
		return nil
	}
}

func memberRemoved(id uint64) func(c *Node) error {
	return func(c *Node) error {
		m, ok := c.GetMemebr(id)
		if ok && m.Type() == RemovedMember {
			return fmt.Errorf("raft: member %d removed", id)
		}
		return nil
	}
}

func addressInUse(mid uint64, addr string) func(c *Node) error {
	return func(c *Node) error {
		membs := c.members(func(m Member) bool {
			return m.Address() == addr && m.ID() != mid && m.Type() != RemovedMember
		})

		if len(membs) > 0 {
			return fmt.Errorf("raft: address used by member %d", membs[0].ID())
		}

		return nil
	}
}

func notLeader() func(c *Node) error {
	return func(c *Node) error {
		if c.Whoami() != c.Leader() {
			return ErrNotLeader
		}
		return nil
	}
}

func leader(id uint64) func(c *Node) error {
	return func(c *Node) error {
		if id == c.Leader() {
			return fmt.Errorf("raft: operation not permitted, member %d is the leader, transfer leadership first", id)
		}
		return nil
	}
}

func idInUse(id uint64) func(c *Node) error {
	return func(c *Node) error {
		if _, ok := c.GetMemebr(id); ok {
			return fmt.Errorf("raft: id used by member %d", id)
		}
		return nil
	}
}

func noLeader() func(c *Node) error {
	return func(c *Node) error {
		if c.Leader() == None {
			return raftengine.ErrNoLeader
		}
		return nil
	}
}

func disableForwarding() func(c *Node) error {
	return func(c *Node) error {
		disable := c.cfg.rcfg.DisableProposalForwarding
		if c.Leader() != c.Whoami() && disable {
			return ErrNotLeader
		}
		return nil
	}
}

func notType(id uint64, t MemberType) func(c *Node) error {
	return func(c *Node) error {
		mem, _ := c.GetMemebr(id)
		if mt := mem.Type(); mt != t {
			return fmt.Errorf("raft: memebr (%d) is a %s not a %s", id, mt, t)
		}
		return nil
	}
}
