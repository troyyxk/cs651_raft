package shardkv

import "6.824/shardctrler"

//
// Sharded key/value server.
// Lots of replica groups, each running Raft.
// Shardctrler decides which group serves each shard.
// Shardctrler may change shard assignment from time to time.
//
// You will have to modify these definitions.
//

type Err string

const (
	OK                 Err = "OK"
	ErrNoKey           Err = "ErrNoKey"
	ErrWrongGroup      Err = "ErrWrongGroup"
	ErrWrongLeader     Err = "ErrWrongLeader"
	ErrNoConfig        Err = "ErrNoConfig"
	ErrUnmatchedConfig Err = "ErrUnmatchedConfig"
	ErrNotUpToDate     Err = "ErrNotUpToDate"
	ErrOther           Err = "ErrOther"
	ErrTimeout         Err = "ErrTimeout"
	ErrFiller          Err = "ErrFiller"
)

type AcquireShardErr string

const (
	ASOK          AcquireShardErr = "ASOK"
	ASAhead       AcquireShardErr = "ASAhead"
	ASBehind      AcquireShardErr = "ASBehind"
	ASWrongLeader AcquireShardErr = "ASWrongLeader"
	ASFiller      AcquireShardErr = "ASFiller"
)

const (
	AcquireShardWaitTime = 50
	ShardConfigWaitTime  = 100
	StartChannelWaitTime = 300
)

type OpType string

const (
	GET                OpType = "GET"
	APPEND             OpType = "APPEND"
	PUT                OpType = "PUT"
	InstallShardConfig OpType = "InstallShardConfig"
	AcquireShard       OpType = "AcquireShard"
)

// Put or Append
type PutAppendArgs struct {
	// You'll have to add definitions here.
	Key   string
	Value string
	Op    OpType // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientId     int64
	RequestIndex int
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key string
	// You'll have to add definitions here.
	ClientId     int64
	RequestIndex int
}

type GetReply struct {
	Err   Err
	Value string
}

type AcquireShardArgs struct {
	Gid        int // for debugging
	ConfigNum  int // config num of the sender
	ShardIndex int
}

type AcquireShardReply struct {
	ConfigNum         int // config num of the receiver
	ShardTimestamp    int
	Error             AcquireShardErr
	ShardData         KeyValueStore
	ClientLastRequest map[int64]int
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ServerId int
	OpIndex  int

	ClientId     int64
	RequestIndex int

	// type of the command
	Type OpType

	// need to send back to reply channel
	NeedSendBackCh bool

	// for put get and append
	Key   string
	Value string

	// for install new shard config
	ShardConfig shardctrler.Config

	// transfer shard
	ShardId           int
	ShardConfigNum    int
	ShardData         KeyValueStore
	ClientLastRequest map[int64]int
}

type KeyValueStore struct {
	KvStore map[string]string
}

type State struct {
	AllConfigs        []shardctrler.Config
	LatestConfig      shardctrler.Config
	ShardKVStore      map[int]KeyValueStore
	ShardTimestamp    map[int]int
	ClientLastRequest map[int64]int
}

type KVReplyMsg struct {
	Error Err
	Value string
}

type OpKVReplyChannel struct {
	Operation    Op
	ReplyChannel chan KVReplyMsg
}
