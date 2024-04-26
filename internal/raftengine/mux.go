package raftengine

import (
	"context"
	"encoding/json"

	"github.com/shaj13/raft/raftlog"
	"go.etcd.io/etcd/raft/v3"
	etcdraftpb "go.etcd.io/etcd/raft/v3/raftpb"
)

const (
	// add nodeState into the multiplexer.
	add operationType = iota
	// remove nodeState from the multiplexer.
	remove
	// call a method on raft raw node.
	call
	// advance raft raw node.
	advance
	// tick raft raw node.
	tick
	// heartbeat fannout heartbeat msg to all raw nodes.
	heartbeat
)

// NewMux return's a new mux.
func NewMux() Mux {
	return &mux{
		operationc: make(chan *operation),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// operationType specifies the type of operation that multiplexer need to performs.
type operationType uint

// callFunc is the type of the function called by multiplexer to call the desired raw node method.
type callFunc func(rn *raft.RawNode) error

// operation define the method by which a multiplexer performs the requested function.
type operation struct {
	// ot specifies the operation type.
	ot operationType
	// gid specifies group id.
	gid uint64
	// value specifies the operation request/reply content.
	value interface{}
	// done notify the caller that mux has process the op.
	done chan struct{}
}

// nodeState represents the internal state of a RawNode object. All variables here
// are accessible only from the mux.start goroutine so they can be accessed without
// synchronization.
type nodeState struct {
	rn     *raft.RawNode
	cfg    *raft.Config
	lead   uint64
	readyc chan raft.Ready
}

// mux represents a multi node state that is participating in multiple consensus groups,
// a mux is more efficient than a collection of nodes.
// the name mux stands for "multiplexer". Like the standard "http.ServeMux".
type mux struct {
	operationc chan *operation
	stop       chan struct{}
	done       chan struct{}
}

func (m *mux) Stop() {
	// send stop signal.
	close(m.stop)
	// wait for mux to be terminated.
	<-m.done
}

func (m *mux) Start() {

	defer close(m.done)
	nodes := map[uint64]*nodeState{}
	advcs := map[uint64]raft.Ready{}
	hb := newHeartbeats()
	ticks := 0

	for {
		if len(advcs) == 0 {
			for gid, n := range nodes {
				st := n.rn.BasicStatus()
				if n.lead != st.Lead {
					if st.Lead != 0 {
						if n.lead == 0 {
							n.cfg.Logger.Infof("raft.node: %d elected leader %d at term %d", st.ID, st.Lead, st.Term)
						} else {
							n.cfg.Logger.Infof("raft.node: %d changed  leader from %d to %d at term %d", st.ID, n.lead, st.Lead, st.Term)
						}
					} else {
						n.cfg.Logger.Infof("raft.node: %d lost leader %d at term %d", st.ID, n.lead, st.Term)
					}
					n.lead = st.Lead
				}

				if n.rn.HasReady() {
					rd := n.rn.Ready()
					advcs[gid] = rd
					n.readyc <- hb.suppress(rd)
				}
			}
		}

		var node *nodeState

		select {
		case op := <-m.operationc:
			node = nodes[op.gid]
			if node == nil && op.ot != add {
				raftlog.Warningf("raft: unknown group %d", op.gid)
				close(op.done)
				continue
			}

			switch op.ot {
			case add:
				node = op.value.(*nodeState)
				current := hb.id
				id := node.cfg.ID
				if id != current && current != raft.None {
					raftlog.Panic("raft: all node group must have the same id !!")
				}
				nodes[op.gid] = node
				hb.id = id
			case remove:
				delete(nodes, op.gid)
				delete(advcs, op.gid)
			case call:
				err := op.value.(callFunc)(node.rn)
				op.value = err
			case advance:
				rd := advcs[op.gid]
				node.rn.Advance(rd)
				delete(advcs, op.gid)
			case heartbeat:
				msg := op.value.(etcdraftpb.Message)
				hb.fanout(nodes, msg)
			case tick:
				ticks++
				node.rn.Tick()
				if (ticks / len(nodes)) >= node.cfg.HeartbeatTick {
					ticks = 0
					hb.coalesced(nodes)
				}
			}

			close(op.done)

		case <-m.stop:
			return
		}
	}
}

func (m *mux) add(gid uint64, rn *raft.RawNode, cfg *raft.Config) raft.Node {
	node := &nodeState{
		rn:     rn,
		cfg:    cfg,
		readyc: make(chan raft.Ready, 128),
	}

	op := &operation{
		ot:    add,
		gid:   gid,
		value: node,
	}

	_ = m.push(context.Background(), op)

	return &muxNode{
		gid:    gid,
		mux:    m,
		readyc: node.readyc,
	}
}

func (m *mux) remove(gid uint64) {
	op := &operation{
		ot:  remove,
		gid: gid,
	}

	_ = m.push(context.Background(), op)
}

func (m *mux) call(ctx context.Context, gid uint64, fn callFunc) error {
	op := &operation{
		ot:    call,
		gid:   gid,
		value: fn,
	}

	if err := m.push(ctx, op); err != nil {
		return err
	}

	if err, ok := op.value.(error); ok {
		return err
	}

	return nil
}

func (m *mux) push(ctx context.Context, op *operation) error {
	op.done = make(chan struct{})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return ErrStopped
	case m.operationc <- op:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return ErrStopped
	case <-op.done:
		return nil
	}
}

func (m *mux) tick(gid uint64) {
	op := &operation{
		ot:  tick,
		gid: gid,
	}
	_ = m.push(context.Background(), op)
}

func (m *mux) campaign(ctx context.Context, gid uint64) error {
	return m.call(ctx, gid, func(rn *raft.RawNode) error {
		return rn.Campaign()
	})
}

func (m *mux) propose(ctx context.Context, gid uint64, data []byte) error {
	return m.call(ctx, gid, func(rn *raft.RawNode) error {
		return rn.Propose(data)
	})
}

func (m *mux) proposeConfChange(ctx context.Context, gid uint64, cc etcdraftpb.ConfChangeI) error {
	return m.call(ctx, gid, func(rn *raft.RawNode) error {
		return rn.ProposeConfChange(cc)
	})
}

func (m *mux) step(ctx context.Context, gid uint64, msg etcdraftpb.Message) error {
	switch msg.Type {
	case etcdraftpb.MsgHeartbeat, etcdraftpb.MsgHeartbeatResp:
		op := &operation{
			ot:    heartbeat,
			gid:   gid,
			value: msg,
		}
		return m.push(ctx, op)
	default:
		return m.call(context.Background(), gid, func(rn *raft.RawNode) error {
			return rn.Step(msg)
		})
	}
}

func (m *mux) advance(gid uint64) {
	op := &operation{
		ot:  advance,
		gid: gid,
	}

	_ = m.push(context.Background(), op)
}

func (m *mux) applyConfChange(gid uint64, cc etcdraftpb.ConfChangeI) *etcdraftpb.ConfState {
	cs := new(etcdraftpb.ConfState)
	_ = m.call(context.Background(), gid, func(rn *raft.RawNode) error {
		cs = rn.ApplyConfChange(cc)
		return nil
	})

	return cs
}

func (m *mux) transferLeadership(ctx context.Context, gid, _, transferee uint64) {
	_ = m.call(ctx, gid, func(rn *raft.RawNode) error {
		rn.TransferLeader(transferee)
		return nil
	})

}

func (m *mux) readIndex(ctx context.Context, gid uint64, rctx []byte) error {
	return m.call(ctx, gid, func(rn *raft.RawNode) error {
		rn.ReadIndex(rctx)
		return nil
	})
}

func (m *mux) status(gid uint64) (st raft.Status) {
	_ = m.call(context.Background(), gid, func(rn *raft.RawNode) error {
		st = rn.Status()
		return nil
	})
	return
}

func (m *mux) reportUnreachable(gid, id uint64) {
	_ = m.call(context.Background(), gid, func(rn *raft.RawNode) error {
		rn.ReportUnreachable(id)
		return nil
	})
}

func (m *mux) reportSnapshot(gid, id uint64, status raft.SnapshotStatus) {
	_ = m.call(context.Background(), gid, func(rn *raft.RawNode) error {
		rn.ReportSnapshot(id, status)
		return nil
	})
}

type muxNode struct {
	readyc <-chan raft.Ready
	gid    uint64
	mux    *mux
}

func (m *muxNode) Tick() {
	m.mux.tick(m.gid)
}

func (m *muxNode) Campaign(ctx context.Context) error {
	return m.mux.campaign(ctx, m.gid)
}

func (m *muxNode) Propose(ctx context.Context, data []byte) error {
	return m.mux.propose(ctx, m.gid, data)
}

func (m *muxNode) ProposeConfChange(ctx context.Context, cc etcdraftpb.ConfChangeI) error {
	return m.mux.proposeConfChange(ctx, m.gid, cc)
}

func (m *muxNode) Step(ctx context.Context, msg etcdraftpb.Message) error {
	return m.mux.step(ctx, m.gid, msg)
}

func (m *muxNode) Ready() <-chan raft.Ready {
	return m.readyc
}

func (m *muxNode) Advance() {
	m.mux.advance(m.gid)
}

func (m *muxNode) ApplyConfChange(cc etcdraftpb.ConfChangeI) *etcdraftpb.ConfState {
	return m.mux.applyConfChange(m.gid, cc)
}

func (m *muxNode) TransferLeadership(ctx context.Context, lead, transferee uint64) {
	m.mux.transferLeadership(ctx, m.gid, lead, transferee)
}

func (m *muxNode) ReadIndex(ctx context.Context, rctx []byte) error {
	return m.mux.readIndex(ctx, m.gid, rctx)
}

func (m *muxNode) Status() raft.Status {
	return m.mux.status(m.gid)
}

func (m *muxNode) ReportUnreachable(id uint64) {
	m.mux.reportUnreachable(m.gid, id)
}

func (m *muxNode) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	m.mux.reportSnapshot(m.gid, id, status)
}

func (m *muxNode) Stop() {
	m.mux.remove(m.gid)
}

func newHeartbeats() *heartbeats {
	return &heartbeats{
		pending: map[string]struct{}{},
		step: func(ns *nodeState, m etcdraftpb.Message) {
			_ = ns.rn.Step(m)
		},
	}
}

// heartbeats represent an asset used by mux to reduce
// heartbeat network traffic by coalesced heartbeats.
// each pair of physical nodes/process only needs to exchange heartbeats once per tick,
// no matter how many raft nodes they have in common.
type heartbeats struct {
	id      uint64
	pending map[string]struct{}
	// abstracted for testing purposes.
	step func(*nodeState, etcdraftpb.Message)
}

func (h *heartbeats) suppress(rd raft.Ready) raft.Ready {
	for i := 0; i < len(rd.Messages); i++ {
		msg := rd.Messages[i]
		switch msg.Type {
		case etcdraftpb.MsgHeartbeat:
			// record heartbeats by context,
			// typically ctx is embty unless there a safe read index request.
			// individual ctx coalesced later.
			// Same as etcd.Raft https://github.com/etcd-io/etcd/blob/main/raft/read_only.go#L68.
			h.pending[string(msg.Context)] = struct{}{}
			fallthrough
		case etcdraftpb.MsgHeartbeatResp:
			// MsgHeartbeatResp must be ignored from being pend, otherwise,
			// the number of heartbeats will be doubled,
			// instead, the fanout should respond accordingly.
			// i.e
			// incorrect flow:
			// node A ->  heartbeats -> node B (coalesced)
			// node B ->  heartbeats -> node A (coalesced)
			// node B ->  heartbeats resp -> node A (fanout)
			// correct flow:
			// node A ->  heartbeats -> node B (coalesced)
			// node B ->  heartbeats resp -> node A (fanout)

			// suppress both MsgHeartbeatResp & MsgHeartbeat.
			// individual heartbeats coalesced later.
			rd.Messages = append(rd.Messages[:i], rd.Messages[i+1:]...)
			i--
		}
	}
	return rd
}

func (h *heartbeats) coalesced(nodes map[uint64]*nodeState) {
	if len(h.pending) == 0 {
		return
	}

	// sent avoid sending the heartbeat to the same node
	// that is participating in multiple consensus groups.
	// i.e Nodes A,B groups C,D
	// node A send msg once to B either in C or D.
	sent := make(map[uint64]struct{})
	// don't heartbeat yourself.
	sent[h.id] = struct{}{}

	cc := new(coalescedContext)
	for ctx := range h.pending {
		cc.Buffers = append(cc.Buffers, []byte(ctx))
	}

	bcc, err := json.Marshal(cc)
	if err != nil {
		raftlog.Warningf("raft: marshal coalesced heartbeats context: %v", err)
	}

	for _, node := range nodes {
		cfg := node.rn.Status().Config
		msgs := []etcdraftpb.Message{}
		for _, v := range []map[uint64]struct{}{
			cfg.Voters.IDs(),
			cfg.Learners,
		} {
			for id := range v {
				if _, ok := sent[id]; ok {
					continue
				}

				sent[id] = struct{}{}

				mh := etcdraftpb.Message{
					From:    h.id,
					To:      id,
					Type:    etcdraftpb.MsgHeartbeat,
					Context: bcc,
				}
				msgs = append(msgs, mh)
			}
		}

		node.readyc <- raft.Ready{
			Messages: msgs,
		}
	}

	// reset the heartbeats state.
	h.pending = map[string]struct{}{}
}

func (h *heartbeats) fanout(nodes map[uint64]*nodeState, msg etcdraftpb.Message) {
	var (
		node    *nodeState
		success bool
	)

	defer func() {
		if !success && raftlog.V(8).Enabled() {
			str := raft.DescribeMessage(msg, nil)
			raftlog.V(8).Infof("raft: not fanning out msg: %s", str)
		}
	}()

	isResp := msg.Type == etcdraftpb.MsgHeartbeatResp
	ctx := msg.Context

	cc := new(coalescedContext)
	if err := json.Unmarshal(ctx, cc); err != nil {
		raftlog.Warningf("raft: unmarshal coalesced heartbeats context: %v", err)
	}

	for _, n := range nodes {
		// ok allow sends the given heartbeat to all groups which believe that
		// their leader resides on the sending node.
		ok := (n.lead == msg.From || n.lead == raft.None) && !isResp
		// resp sends the given heartbeat response to all groups
		// which overlap with the sender's groups and consider themselves leader.
		okResp := n.lead == h.id && isResp

		// assign a node to reply to the given heartbeat.
		// the assigned node must recognize the sender.
		//
		// Note: when a node is barely fresh the leader id may not exist in the conf state.
		_, found := n.rn.Status().Config.Voters.IDs()[msg.From]
		if ok || found {
			node = n
		}

		if !(ok || okResp) {
			continue
		}

		for _, ctx := range cc.Buffers {
			m := etcdraftpb.Message{
				From:    msg.From,
				To:      msg.To,
				Type:    msg.Type,
				Context: ctx,
			}
			h.step(n, m)
		}

		success = true
	}

	if isResp {
		return
	}

	if node == nil {
		raftlog.Warningf("raft: ignored heartbeat from unknown member %d", msg.From)
		return
	}

	// must respond whether the msg forwarded to any groups or not.
	node.readyc <- raft.Ready{
		Messages: []etcdraftpb.Message{
			{
				From:    msg.To,
				To:      msg.From,
				Type:    etcdraftpb.MsgHeartbeatResp,
				Context: ctx,
			},
		},
	}
}

// coalescedContext hold heartbeats context if any.
type coalescedContext struct {
	Buffers [][]byte
}
