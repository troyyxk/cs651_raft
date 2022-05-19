package shardctrler

//
// Shard controler: assigns shards to replication groups.
//
// RPC interface:
// Join(servers) -- add a set of groups (gid -> server-list mapping).
// Leave(gids) -- delete a set of groups.
// Move(shard, gid) -- hand off one shard from current owner to gid.
// Query(num) -> fetch Config # num, or latest config if num==-1.
//
// A Config (configuration) describes a set of replica groups, and the
// replica group responsible for each shard. Configs are numbered. Config
// #0 is the initial configuration, with no groups and all shards
// assigned to group 0 (the invalid group).
//
// You will need to add fields to the RPC argument structs.
//

// The number of shards.
const NShards = 10

// A configuration -- an assignment of shards to groups.
// Please don't change this.
type Config struct {
	Num    int              // config number
	Shards [NShards]int     // shard -> gid
	Groups map[int][]string // gid -> servers[]
}

const (
	StartChannelWaitTime = 300
)

type SCtrlType string

const (
	JOIN  SCtrlType = "JOIN"
	LEAVE SCtrlType = "LEAVE"
	MOVE  SCtrlType = "MOVE"
	QUERY SCtrlType = "QUERY"
)

type ErrType string

const (
	OK             ErrType = "OK"
	ErrNoKey       ErrType = "ErrNoKey"
	ErrWrongLeader ErrType = "ErrWrongLeader"
	ErrOther       ErrType = "ErrOther"
	ErrTimeout     ErrType = "ErrTimeout"
	ErrFiller      ErrType = "ErrFiller"
)

type Op struct {
	// Your data here.
	ServerId int
	OpIndex  int

	// type of the command
	Type SCtrlType
	// join
	Servers map[int][]string
	// leave
	GIDs []int
	// move
	Shard int
	GID   int
	// query
	Num int

	ClientId     int64
	RequestIndex int
}

type JoinArgs struct {
	Servers map[int][]string // new GID -> servers mappings

	ClientId     int64
	RequestIndex int
}

type JoinReply struct {
	WrongLeader bool
	Err         ErrType
}

type LeaveArgs struct {
	GIDs []int

	ClientId     int64
	RequestIndex int
}

type LeaveReply struct {
	WrongLeader bool
	Err         ErrType
}

type MoveArgs struct {
	Shard int
	GID   int

	ClientId     int64
	RequestIndex int
}

type MoveReply struct {
	WrongLeader bool
	Err         ErrType
}

type QueryArgs struct {
	Num int // desired config number

	ClientId     int64
	RequestIndex int
}

type QueryReply struct {
	WrongLeader bool
	Err         ErrType
	Config      Config
}

type ReplyMsg struct {
	Error        ErrType
	Value        string
	ReturnConfig Config
}

type OpKVReplyChannel struct {
	Operation    Op
	ReplyChannel chan ReplyMsg
}

type State struct {
	Configs           []Config
	ClientLastRequest map[int64]int
}
