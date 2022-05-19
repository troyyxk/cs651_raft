package kvraft

import "log"

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
	ErrOther       = "ErrOther"
	ErrTimeout     = "ErrTimeout"
	ErrFiller      = "ErrFiller"
)

type Err string

const (
	StartChannelWaitTime = 300
)

type OpType string

const (
	GET    OpType = "GET"
	APPEND OpType = "APPEND"
	PUT    OpType = "PUT"
)

type Op struct {
	// id/key of this Op involve both server id and op index
	// ServerId == kv.me
	// OpIndex will increase every time an Op has been created
	ServerId int
	OpIndex  int

	// type of the command
	Type OpType

	Key   string
	Value string

	ClientId     int64
	RequestIndex int
}

type KVReplyMsg struct {
	Error Err
	Value string
}

type OpKVReplyChannel struct {
	Operation    Op
	ReplyChannel chan KVReplyMsg
}

func (kv *KVServer) getValue(op Op) (string, Err) {
	if op.Type != GET {
		log.Fatalln("In geValue(), op type not GET")
	}
	if val, ok := kv.state.KeyValueStore[op.Key]; ok {
		return val, OK
	} else {
		return "", ErrNoKey
	}
}
