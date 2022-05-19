package kvraft

import (
	"log"
	"time"
)

// ----------- Struct -----------

// Put or Append
type PutAppendArgs struct {
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

// ----------- Client Side -----------

func (ck *Clerk) createPutAppendArgs(key string, value string, op OpType) PutAppendArgs {
	args := PutAppendArgs{}
	args.Key = key
	args.Value = value
	args.Op = op
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	return args
}

func (ck *Clerk) sendPutAppend(server int, args *PutAppendArgs, reply *PutAppendReply) bool {
	ok := ck.servers[server].Call("KVServer.PutAppend", args, reply)
	return ok
}

//
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op OpType) {
	// You will have to modify this function.
	ck.requestIndex++

	for {
		curServer := ck.lastLeaderServer
		DPrintf("PPP> ck.clerkRandomId: %v, key: %v, value: %v, target server: %v, send %v request.\n",
			ck.clerkRandomId, key, value, curServer,
			op)
		// send get
		args := ck.createPutAppendArgs(key, value, op)
		reply := PutAppendReply{}
		ok := ck.sendPutAppend(curServer, &args, &reply)

		// handle args
		if !ok {
			DPrintf("PPP> ck.clerkRandomId: %v, key: %v, value: %v, target server: %v, connection not ok.\n",
				ck.clerkRandomId, key, value, curServer)
			// enter a new loop with trying another next server
			ck.lastLeaderServer = (curServer + 1) % len(ck.servers)
			continue
		}

		// connection ok
		// result successfully fetched, return the result
		if reply.Err == OK {
			DPrintf("PPP> ck.clerkRandomId: %v, key: %v, value: %v, arget server: %v, kv pair %v success according to reply.\n",
				ck.clerkRandomId, key, value, curServer, op)
			return
		}

		// for all other errors, retry with the next server
		DPrintf("PPP> ck.clerkRandomId: %v, key: %v, value: %v, target server: %v, get request return with ERR: %v, retry with the next server.\n",
			ck.clerkRandomId, key, value, curServer,
			reply.Err)

		// enter a new loop with trying another next server
		ck.lastLeaderServer = (curServer + 1) % len(ck.servers)
	}

}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, PUT)
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, APPEND)
}

// ----------- Server Side -----------

func (kv *KVServer) createPutAppendOp(args *PutAppendArgs) Op {
	op := Op{}
	op.ServerId = kv.me
	op.OpIndex = kv.opIndex

	// everytime opIndex is assigned, it increases
	kv.opIndex++

	op.Type = args.Op
	op.Key = args.Key
	op.Value = args.Value

	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

// return op and whether to stop
func (kv *KVServer) prePutAppendStart(args *PutAppendArgs, reply *PutAppendReply) (Op, OpKVReplyChannel, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SPP> kvserver: %v, key: %v, value: %v, trying to : %v, kv.state.keyValueStore: %v.\n",
		kv.me, args.Key, args.Value, args.Op, kv.state.KeyValueStore)

	// create the op for this get
	op := kv.createPutAppendOp(args)

	//if kv.isDuplicateOp(op) {
	//	DPrintf("SPP> kvserver: %v, key: %v, value: %v, duplicate op, op.ClientId: %v, op.RequestIndex: %v, kv.state.clientLastRequest: %v.\n",
	//		kv.me, args.Key, args.Value, op.ClientId, op.RequestIndex, kv.state.clientLastRequest)
	//}

	// else start listen on channel and start the op

	if kv.killed() {
		DPrintf("SPP> kvserver: %v, key: %v, value: %v, attempting to access a killed server, trying to : %v, kv.state.keyValueStore: %v.\n",
			kv.me, args.Key, args.Value, args.Op, kv.state.KeyValueStore)
		reply.Err = ErrWrongLeader
		return op, OpKVReplyChannel{}, true
	}

	DPrintf("SPP> kvserver: %v, key: %v, value: %v, trying to : %v, start raft, op: %v.\n",
		kv.me, args.Key, args.Value, args.Op, op)

	startCh := kv.createStartCh(op)
	return op, startCh, false
}

func (kv *KVServer) handlePutAppendResult(op Op, responseMsg KVReplyMsg, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, responseMsg: %v\n",
		kv.me, responseMsg)

	kv.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handlePutAppendResult(), response error is ErrFiller, which should not be")
	}

	reply.Err = responseMsg.Error
}

func (kv *KVServer) putAppendTimeout(op Op, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SPP> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		kv.me, op.ClientId, op.OpIndex)

	kv.deleteStartCh(op)

	reply.Err = ErrTimeout
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	// if not leader return
	if _, isLeader := kv.rf.GetState(); !isLeader {
		reply.Err = ErrWrongLeader
		return
	}

	op, startCh, toStopPutAppend := kv.prePutAppendStart(args, reply)

	// if already get the value with the key or not leader
	if toStopPutAppend {
		return
	}

	// set error first
	reply.Err = ErrOther

	//kv.rf.Start(op)
	_, _, isLeader := kv.rf.Start(op)
	if !isLeader {
		reply.Err = ErrWrongLeader
		kv.deleteStartChLocked(op)
		return
	}

	DPrintf("SPP> is leader, send op.")

	select {
	case responseMsg := <-startCh.ReplyChannel:
		kv.handlePutAppendResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		kv.putAppendTimeout(op, reply)
		return
	}

}
