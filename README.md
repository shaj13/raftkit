[![GoDoc](https://godoc.org/github.com/shaj13/raft?status.svg)](https://pkg.go.dev/github.com/shaj13/raft)
[![Go Report Card](https://goreportcard.com/badge/github.com/shaj13/raft)](https://goreportcard.com/report/github.com/shaj13/raft)
[![Coverage Status](https://coveralls.io/repos/github/shaj13/raft/badge.svg?branch=master)](https://coveralls.io/github/shaj13/raft?branch=master)
[![CircleCI](https://circleci.com/gh/shaj13/raft/tree/main.svg?style=svg)](https://circleci.com/gh/shaj13/raft/tree/main)

## Overview 
Raft is a protocol with which a cluster of nodes can maintain a replicated state machine.
The state machine is kept in sync through the use of a replicated log.
However, The details of the Raft protocol are outside the scope of this document,
For more details on Raft, see [In Search of an Understandable Consensus Algorithm](https://raft.github.io/raft.pdf)


## Why another library? 
Raft algorithm comes in search of an understandable consensus algorithm, unfortunately, most of the go libraries out there required a deep knowledge of their implementation and APIs. 

This raft library was born to align with the understandability raft principle and its sole purpose is to provide consensus with the minimalistic, simple, clean, and idiomatic API. 

Etcd Raft is the most widely used Raft library in production
But, it follows a minimalistic design philosophy by only implementing the core raft algorithm which leaves gaps and ambiguities.

So, instead of reinventing the wheel, this library uses etcd raft as its core.

That's how you can benefit from the power and stability of etcd raft, with an understandable API. indeed, it keeps your focus on building awesome software. 

Finally, the raft library aimed to be used internally but it is worth being exposed to the public.

## Features

This raft implementation is a full feature implementation of Raft protocol. Features includes:

- Mange Multi-Raft 
- Coalesced heartbeats to reduce the overhead of heartbeats when there are a large number of raft groups
- Leader election
- Log replication
- Log compaction
- Pre-Vote Protocol 
- Membership changes
  - add member 
  - remove member
  - update member 
  - promote member
  - demote member
- Leadership transfer extension
- Efficient linearizable read-only queries served by both the leader and followers
  - leader checks with quorum and bypasses Raft log before processing read-only queries
  - followers asks leader to get a safe read index before processing read-only queries
- More efficient lease-based linearizable read-only queries served by both the leader and followers
  - leader bypasses Raft log and processing read-only queries locally
  - followers asks leader to get a safe read index before processing read-only queries
  - this approach relies on the clock of the all the machines in raft group
- Snapshots 
  - automatic snapshots when the log store reaches a certain length 
  - API to force new snapshot 
- Read-Only Members
  - learner member 
  - staging member
- Segmented WAL to provide durability and ensure data integrity
- Garbage collector to controls how many WAL and snapshot files are retained
- Network transport to communicate with Raft on remote machines
  - gRPC (recommended)
  - http 
- gRPC chunked transfer encoding
- Network [Pipelining](https://developer.mozilla.org/en-US/docs/Web/HTTP/Connection_management_in_HTTP_1.x#http_pipelining)
- Restore cluster quorum and data 
  - force new cluster from the existing WALL and snapshot
  - restore the cluster from snapshot
- Optimistic pipelining to reduce log replication latency
- Flow control for log replication
- Batching Raft messages to reduce synchronized network I/O calls
- Batching log entries to reduce disk synchronized I/O
- Writing to leader's disk in parallel
- Internal proposal redirection from followers to leader
- Automatic stepping down when the leader loses quorum
- Protection against unbounded log growth when quorum is lost

## WAL's and snapshots

There are two sets of files on disk that provide persistent state for Raft.
There is a set of WAL (write-ahead log files). These store a series of log
entries and Raft metadata, such as the current term, index, and committed index.
WAL files are automatically rotated when they reach a certain size.

To avoid having to retain every entry in the history of the log, snapshots
serialize a view of the state at a particular point in time. After a snapshot
gets taken, logs that predate the snapshot are no longer necessary, because the
snapshot captures all the information that's needed from the log up to that
point. The number of old snapshots and WALs to retain is configurable.

WALs mostly contain protobuf-serialized user data store
modifications. A log entry can contain a batch of creations, updates, and
deletions of objects from the user data store. Some log entries contain other kinds
of metadata, like node additions or removals. Snapshots contain a complete dump
of the store, as well as any metadata from the log entries that needs to be
preserved. The saved metadata includes the Raft term and index, a list of nodes
in the cluster, and a list of nodes that have been removed from the cluster.

## Raft IDs
The library uses integers to identify Raft nodes. The Raft IDs may assigned dynamically 
when a node joins the Raft consensus group, or it can be defined manually by the user.

It's important to note that a Raft ID can't be reused after a node that was
using the ID leaves the consensus group. These Raft IDs of nodes that are no
longer part of the cluster are saved (persisted on disk) as part of the nodes
pool members to make sure they aren't reused. If a node with a removed Raft ID 
tries to use Raft RPCs, other nodes won't honor these requests.

The removed node's IDs are used to restrict these nodes from
communicating, affecting the cluster state and avoid ambiguity.

## Initializing a Raft cluster
The first member of a cluster assigns itself a random Raft ID unless it pre-defined.
It creates a new WAL with its own Raft identity stored in the metadata field.
The metadata field is the only part of the WAL that differs between nodes. By
storing information such as the local Raft ID, it's easy to restore this
node-specific information after a restart. In principle it could be stored in a
separate file, but embedding it inside the WAL is most convenient.

The node then starts the Raft state machine. From this point, it's a fully
functional single-node Raft instance. Writes to the data store actually go
through Raft, though this is a trivial case because reaching consensus doesn't
involve communicating with any other nodes.

## Joining a Raft cluster 
New nodes can join an existing Raft consensus group by invoking the `Join` RPC
on any Raft member if proposal forwarding is enabled, Otherwise,
`Join` RPC must be invoked on the leader. If successful, `Join` returns a
Raft ID for the new node and a list of other members of the consensus group.

On the leader side, `Join` tries to append a configuration change entry to the
Raft log, and waits until that entry becomes committed.

A new node creates an empty Raft log with its own node information in the
metadata field. Then it starts the state machine. By running the Raft consensus
protocol, the leader will discover that the new node doesn't have any entries in
its log, and will synchronize these entries to the new node through some
combination of sending snapshots and log entries. It can take a little while for
a new node to become a functional member of the consensus group, because it
needs to receive this data first.

The new node can join the cluster as a Voter, Learner, or Staging member.

## Initializing a predefined Raft cluster
The library also provides a mechanism to initialize and boot the Raft cluster
from a predefined members configuration. This is done by applying the same 
configurations to all members even for the later joining member. 

Each member of the cluster use its predefined id and creates a new 
WAL with its own Raft identity stored in the metadata field. The member
node then starts the Raft state machine.

Once all members nodes are started the election process will be initiated,
and when all members agrees on the same leader the cluster becomes fully functional,
Writes to the data store actually go through Raft and it considered complete after reaching a majority.

## Usage 
The primary object in raft is a Node. Either start a Node from scratch using `raft.WithInitCluster()`, `raft.WithJoin()` or start a Node from some initial state using `raft.WithRestart()`.

To start a three-node cluster from predefined configuration: 

**Node A** 
```go
m1 := raft.RawMember{ID: 1, Address: ":8081"}
m2 := raft.RawMember{ID: 2, Address: ":8082"}
m3 := raft.RawMember{ID: 3, Address: ":8083"}
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
// The first member should reference to the current effective member.
node.Start(raft.WithInitCluster(), raft.WithMembers(m1, m2, m3))
```

**Node B** 
```go
m1 := raft.RawMember{ID: 1, Address: ":8081"}
m2 := raft.RawMember{ID: 2, Address: ":8082"}
m3 := raft.RawMember{ID: 3, Address: ":8083"}
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
// The first member should reference to the current effective member.
node.Start(raft.WithInitCluster(), raft.WithMembers(m2, m1, m3))
```

**Node C** 
```go
m1 := raft.RawMember{ID: 1, Address: ":8081"}
m2 := raft.RawMember{ID: 2, Address: ":8082"}
m3 := raft.RawMember{ID: 3, Address: ":8083"}
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
// The first member should reference to the current effective member.
node.Start(raft.WithInitCluster(), raft.WithMembers(m3, m1, m2))
```


Start a single node cluster, like so:
```go
m := raft.RawMember{ID: 1, Address: ":8081"}
opt := raft.WithMembers(m)
// or 
opt = raft.WithAddress(":8081")
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
node.Start(raft.WithInitCluster(), opt)
```

To allow a new node to join a cluster, like so:
```go
m := raft.RawMember{ID: 2, Address: ":8082"}
opt := raft.WithMembers(m)
// or 
opt = raft.WithAddress(":8082")
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
node.Start(raft.WithJoin(":8081", time.Second), opt)
```

To restart a node from previous state:
```go
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
node.Start(raft.WithRestart())
```

To force new cluster:
```go
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
// This will use the latest wal and snapshot.
node.Start(raft.WithForceNewCluster())
```

To restore from snapshot: 
```go 
node := raft.NewNode(<FSM>, <Transport>, <Opts>)
node.Start(raft.WithRestore("<path to snapshot file>"))
```

## Examples and docs
 - More detailed development documentation can be found in [go docs](https://pkg.go.dev/github.com/shaj13/raft)
 - Fully working single and multiraft cluster example can be found in [Examples Folder](./_examples).

## Contributing to this project
We welcome contributions. If you find any bugs, potential flaws and edge cases, improvements, new feature suggestions or discussions, please submit issues or pull requests.






