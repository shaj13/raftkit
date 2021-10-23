package raft

import (
	"context"
	"os"
	"time"

	"github.com/shaj13/raftkit/internal/daemon"
	"github.com/shaj13/raftkit/internal/log"
	"github.com/shaj13/raftkit/internal/membership"
	"github.com/shaj13/raftkit/internal/rpc"
	"github.com/shaj13/raftkit/internal/storage"
	"go.etcd.io/etcd/raft/v3"
)

// URL return's string represents a member URL.
// The general form represented is:
//
//	{id}={addr}
//
var MemberURL = membership.URL

// Logger represents an active logging object that generates lines of
// output to an io.Writer.
type Logger = log.Logger

// Option configures raft node using the functional options paradigm popularized by Rob Pike and Dave Cheney.
// If you're unfamiliar with this style,
// see https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html and
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis.
type Option interface {
	apply(c *config)
}

// StartOption configures how we start the raft node using the functional options paradigm popularized by Rob Pike and Dave Cheney.
// If you're unfamiliar with this style,
// see https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html and
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis.
type StartOption interface {
	apply(c *startConfig)
}

// startOptionFunc implements StartOption interface.
type startOptionFunc func(c *startConfig)

// apply the configuration to the provided config.
func (fn startOptionFunc) apply(c *startConfig) {
	fn(c)
}

// OptionFunc implements Option interface.
type optionFunc func(c *config)

// apply the configuration to the provided config.
func (fn optionFunc) apply(c *config) {
	fn(c)
}

// WithLogger sets logger that is used to generates lines of output.
func WithLogger(lg Logger) Option {
	return optionFunc(func(c *config) {
		log.SetLogger(lg)
	})
}

// WithTickInterval is the time interval to,
// increments the internal logical clock for,
// the current raft member by a single tick.
//
// Default Value: 100'ms.
func WithTickInterval(d time.Duration) Option {
	return optionFunc(func(c *config) {
		c.tickInterval = d
	})
}

// WithStreamTimeOut is the timeout on the streaming messages to other raft members.
//
// Default Value: 10's.
func WithStreamTimeOut(d time.Duration) Option {
	return optionFunc(func(c *config) {
		c.streamTimeOut = d
	})
}

// WithDrainTimeOut is the timeout on the streaming pending messages to other raft members.
// Drain can be very useful for graceful shutdown.
//
// Default Value: 10's.
func WithDrainTimeOut(d time.Duration) Option {
	return optionFunc(func(c *config) {
		c.drainTimeOut = d
	})
}

// WithStateDIR is the directory to store durable state (WAL logs and Snapshots).
//
// Default Value: os.TempDir().
func WithStateDIR(dir string) Option {
	return optionFunc(func(c *config) {
		c.statedir = dir
	})
}

// WithMaxSnapshotFiles is the number of snapshots to keep beyond the
// current snapshot.
//
// Default Value: 5.
func WithMaxSnapshotFiles(max int) Option {
	return optionFunc(func(c *config) {
		c.maxSnapshotFiles = max
	})
}

// WithSnapshotInterval is the number of log entries between snapshots.
//
// Default Value: 1000.
func WithSnapshotInterval(i uint64) Option {
	return optionFunc(func(c *config) {
		c.snapInterval = i
	})
}

// WithElectionTick is the number of Node.Tick invocations that must pass between
// elections. That is, if a follower does not receive any message from the
// leader of current term before ElectionTick has elapsed, it will become
// candidate and start an election. ElectionTick must be greater than
// HeartbeatTick. We suggest ElectionTick = 10 * HeartbeatTick to avoid
// unnecessary leader switching.
//
// Default Value: 10.
func WithElectionTick(tick int) Option {
	return optionFunc(func(c *config) {
		c.rcfg.ElectionTick = tick
	})
}

// WithHeartbeatTick is the number of Node.Tick invocations that must pass between
// heartbeats. That is, a leader sends heartbeat messages to maintain its
// leadership every HeartbeatTick ticks.
//
// Default Value: 1.
func WithHeartbeatTick(tick int) Option {
	return optionFunc(func(c *config) {
		c.rcfg.HeartbeatTick = tick
	})
}

// WithMaxSizePerMsg limits the max byte size of each append message. Smaller
// value lowers the raft recovery cost(initial probing and message lost
// during normal operation). On the other side, it might affect the
// throughput during normal replication. Note: math.MaxUint64 for unlimited,
// 0 for at most one entry per message.
//
// Default Value: 1024 * 1024.
func WithMaxSizePerMsg(max uint64) Option {
	return optionFunc(func(c *config) {
		c.rcfg.MaxSizePerMsg = max
	})
}

// WithMaxCommittedSizePerReady limits the size of the committed entries which
// can be applied.
//
// Default Value: 0.
func WithMaxCommittedSizePerReady(max uint64) Option {
	return optionFunc(func(c *config) {
		c.rcfg.MaxCommittedSizePerReady = max
	})
}

// WithMaxUncommittedEntriesSize limits the aggregate byte size of the
// uncommitted entries that may be appended to a leader's log. Once this
// limit is exceeded, proposals will begin to return ErrProposalDropped
// errors. Note: 0 for no limit.
//
// Default Value: 1 << 30.
func WithMaxUncommittedEntriesSize(max uint64) Option {
	return optionFunc(func(c *config) {
		c.rcfg.MaxUncommittedEntriesSize = max
	})
}

// WithMaxInflightMsgs limits the max number of in-flight append messages during
// optimistic replication phase. The application transportation layer usually
// has its own sending buffer over TCP/UDP. Setting MaxInflightMsgs to avoid
// overflowing that sending buffer.
//
// Default Value: 256.
func WithMaxInflightMsgs(max int) Option {
	return optionFunc(func(c *config) {
		c.rcfg.MaxInflightMsgs = max
	})
}

// WithCheckQuorum specifies if the leader should check quorum activity. Leader
// steps down when quorum is not active for an electionTimeout.
//
// Default Value: false.
func WithCheckQuorum() Option {
	return optionFunc(func(c *config) {
		c.rcfg.CheckQuorum = true
	})
}

// WithPreVote enables the Pre-Vote algorithm described in raft thesis section
// 9.6. This prevents disruption when a node that has been partitioned away
// rejoins the cluster.
//
// Default Value: false.
func WithPreVote() Option {
	return optionFunc(func(c *config) {
		c.rcfg.PreVote = true
	})
}

// WithDisableProposalForwarding set to true means that followers will drop
// proposals, rather than forwarding them to the leader. One use case for
// this feature would be in a situation where the Raft leader is used to
// compute the data of a proposal, for example, adding a timestamp from a
// hybrid logical clock to data in a monotonically increasing way. Forwarding
// should be disabled to prevent a follower with an inaccurate hybrid
// logical clock from assigning the timestamp and then forwarding the data
// to the leader.
//
// Default Value: false.
func WithDisableProposalForwarding() Option {
	return optionFunc(func(c *config) {
		c.rcfg.DisableProposalForwarding = true
	})
}

// WithContext set raft node parent ctx, The provided ctx must be non-nil.
//
// The context controls the entire lifetime of the raft node:
// obtaining a connection, sending the msgs, reading the response, and process msgs.
//
// Default Value: context.Background().
func WithContext(ctx context.Context) Option {
	return optionFunc(func(c *config) {
		c.ctx = ctx
	})
}

// WithJoin send rpc request to join an existing cluster.
func WithJoin(addr string, timeout time.Duration) StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.Join(addr, timeout)
		c.appendOperator(opr)
	})
}

// WithForceJoin send rpc request to join an existing cluster even if already part of a cluster.
func WithForceJoin(addr string, timeout time.Duration) StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.ForceJoin(addr, timeout)
		c.appendOperator(opr)
	})
}

// WithInitCluster initialize a new cluster and create first raft node.
func WithInitCluster() StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.InitCluster()
		c.appendOperator(opr)
	})
}

// WithForceNewCluster initialize a new cluster from state dir. One use case for
// this feature would be in restoring cluster quorum.
//
// Note: ForceNewCluster preserve the same node id.
func WithForceNewCluster() StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.ForceNewCluster()
		c.appendOperator(opr)
	})
}

// WithRestore initialize a new cluster from snapshot file. One use case for
// this feature would be in restoring cluster data.
func WithRestore(path string) StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.Restore(path)
		c.appendOperator(opr)
	})
}

// WithRestart restart raft node from state dir.
func WithRestart() StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.Restart()
		c.appendOperator(opr)
	})
}

// WithMembers add the given members to the raft node.
// use raft.MemberURL as input:
//
//  WithMembers(raft.MemberURL(<id>,"<addr>"), ....)
//
// WithMembers safe to be used with initiate cluster kind options,
// ("WithForceNewCluster", "WithRestore", "WithInitCluster")
// Otherwise, it may conflicts with other options like WithJoin.
//
// As long as only one url given, WithMembers will only set the current node id and address,
// then it will be safe to be composed with other options even "WithJoin".
//
// WithMembers and WithInitCluster must be applied to all cluster nodes when they are composed,
// Otherwise, the quorum will be lost and the cluster become unavailable.
//
//  Node A:
//  n.Start(WithInitCluster(), WithMembers(<node A url>, <node B url>))
//
//  Node B:
//  n.Start(WithInitCluster(), WithMembers(<node B url>, <node A url>))
//
// Note: first URL will be assigned to the current node.
func WithMembers(urls ...string) StartOption {
	return startOptionFunc(func(c *startConfig) {
		opr := daemon.Members(urls...)
		c.appendOperator(opr)
	})
}

// WithAddress set the raft node address.
func WithAddress(addr string) StartOption {
	return startOptionFunc(func(c *startConfig) {
		c.addr = addr
	})
}

// WithFallback can be used if other options do not succeed.
//
// 	WithFallback(
//		WithJoin(),
//		WithRestart,
//	)
//
func WithFallback(opts ...StartOption) StartOption {
	return startOptionFunc(func(c *startConfig) {
		// create new startConfig annd apply all opts,
		// then copy all operators to fallback.
		nc := new(startConfig)
		nc.apply(opts...)

		opr := daemon.Fallback(nc.operators...)
		c.appendOperator(opr)
	})
}

type startConfig struct {
	operators []daemon.Operator
	addr      string
}

func (c *startConfig) appendOperator(opr daemon.Operator) {
	c.operators = append(c.operators, opr)
}

func (c *startConfig) apply(opts ...StartOption) {
	for _, opt := range opts {
		opt.apply(c)
	}
}

type config struct {
	ctx              context.Context
	rcfg             *raft.Config
	tickInterval     time.Duration
	streamTimeOut    time.Duration
	drainTimeOut     time.Duration
	statedir         string
	maxSnapshotFiles int
	snapInterval     uint64
	controller       rpc.Controller
	storage          storage.Storage
	pool             membership.Pool
	dial             rpc.Dial
	daemon           daemon.Daemon
}

func (c *config) TickInterval() time.Duration {
	return c.tickInterval
}

func (c *config) StreamTimeout() time.Duration {
	return c.streamTimeOut
}

func (c *config) DrainTimeout() time.Duration {
	return c.drainTimeOut
}

func (c *config) Snapshotter() storage.Snapshotter {
	return c.storage.Snapshotter()
}

func (c *config) StateDir() string {
	return c.statedir
}

func (c *config) MaxSnapshotFiles() int {
	return c.maxSnapshotFiles
}

func (c *config) Controller() rpc.Controller {
	return c.controller
}

func (c *config) Storage() storage.Storage {
	return c.storage
}

func (c *config) SnapInterval() uint64 {
	return c.snapInterval
}

func (c *config) RaftConfig() *raft.Config {
	return c.rcfg
}

func (c *config) Pool() membership.Pool {
	return c.pool
}

func (c *config) Dial() rpc.Dial {
	return c.dial
}

func (c *config) Reporter() membership.Reporter {
	return c.daemon
}

func (c *config) FSM() daemon.FSM {
	return nil
}

func newConfig(opts ...Option) *config {
	c := &config{
		rcfg: &raft.Config{
			ElectionTick:              10,
			HeartbeatTick:             1,
			MaxSizePerMsg:             1024 * 1024,
			MaxInflightMsgs:           256,
			MaxUncommittedEntriesSize: 1 << 30,
		},
		ctx:              context.Background(),
		tickInterval:     time.Millisecond * 100,
		streamTimeOut:    time.Second * 10,
		drainTimeOut:     time.Second * 10,
		maxSnapshotFiles: 5,
		snapInterval:     1000,
		statedir:         os.TempDir(),
	}

	for _, opt := range opts {
		opt.apply(c)
	}

	c.rcfg.Logger = log.GetLogger()

	return c
}
