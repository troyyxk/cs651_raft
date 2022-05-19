package kvraft

import (
	"log"
	"time"
)

// ----------- Struct -----------

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

// ----------- Client Side -----------

func (ck *Clerk) createGetArgs(key string) GetArgs {
	args := GetArgs{}
	args.Key = key
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	return args
}

func (ck *Clerk) sendGet(server int, args *GetArgs, reply *GetReply) bool {
	ok := ck.servers[server].Call("KVServer.Get", args, reply)
	return ok
}

//
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
// todo, what to do when connection not ok
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {
	ck.requestIndex++

	for {
		curServer := ck.lastLeaderServer
		DPrintf("GGG> ck.clerkRandomId: %v, key: %v, target server: %v, send get request.\n",
			ck.clerkRandomId, key, curServer)
		// send get
		args := ck.createGetArgs(key)
		reply := GetReply{}
		ok := ck.sendGet(curServer, &args, &reply)

		// handle args
		if !ok {
			DPrintf("GGG> ck.clerkRandomId: %v, key: %v, target server: %v, connection not ok.\n",
				ck.clerkRandomId, key, curServer)
			// enter a new loop with trying another next server
			ck.lastLeaderServer = (curServer + 1) % len(ck.servers)
			continue
		}

		// connection ok
		// result successfully fetched, return the result
		if reply.Err == OK {
			DPrintf("GGG> ck.clerkRandomId: %v, key: %v, target server: %v, kv pair found, value: %v.\n",
				ck.clerkRandomId, key, curServer, reply.Value)
			return reply.Value
		}
		// kv pair with key does not exist, return empty string
		if reply.Err == ErrNoKey {
			DPrintf("GGG> ck.clerkRandomId: %v, key: %v, target server: %v, kv pair with the given key not found.\n",
				ck.clerkRandomId, key, curServer)
			return ""
		}

		// for all other errors, retry with the next server
		DPrintf("GGG> ck.clerkRandomId: %v, key: %v, target server: %v, get request return with ERR: %v, retry with the next server.\n",
			ck.clerkRandomId, key, curServer,
			reply.Err)

		// enter a new loop with trying another next server
		ck.lastLeaderServer = (curServer + 1) % len(ck.servers)
	}
}

// ----------- Server Side -----------

func (kv *KVServer) createGetOp(args *GetArgs) Op {
	op := Op{}
	op.ServerId = kv.me
	op.OpIndex = kv.opIndex

	// everytime opIndex is assigned, it increases
	kv.opIndex++

	op.Type = GET
	op.Key = args.Key

	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

// return op and whether to stop
func (kv *KVServer) preGetStart(args *GetArgs, reply *GetReply) (Op, OpKVReplyChannel, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SGG> kvserver: %v, key: %v, trying to get, kv.state.keyValueStore: %v.\n",
		kv.me, args.Key, kv.state.KeyValueStore)

	// create the op for this get
	op := kv.createGetOp(args)

	if kv.killed() {
		DPrintf("SGG> kvserver: %v, key: %v, attempting to access a killed server, trying to  get, kv.state.keyValueStore: %v.\n",
			kv.me, args.Key, kv.state.KeyValueStore)
		reply.Err = ErrWrongLeader
		return op, OpKVReplyChannel{}, true
	}

	DPrintf("SGG> kvserver: %v, key: %v, not found key in local store, start raft, op: %v.\n",
		kv.me, args.Key, op)

	startCh := kv.createStartCh(op)
	return op, startCh, false
}

func (kv *KVServer) handleGetResult(op Op, responseMsg KVReplyMsg, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, responseMsg: %v\n",
		kv.me, responseMsg)

	kv.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleGetResult(), response error is ErrFiller, which should not be")
	}

	reply.Err, reply.Value = responseMsg.Error, responseMsg.Value
}

func (kv *KVServer) getTimeout(op Op, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("SGG> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		kv.me, op.ClientId, op.OpIndex)

	kv.deleteStartCh(op)

	reply.Err = ErrTimeout
}

func (kv *KVServer) Get(args *GetArgs,
	reply *GetReply) {
	// Your code here.
	// if not leader return
	if _, isLeader := kv.rf.GetState(); !isLeader {
		reply.Err = ErrWrongLeader
		return
	}

	op, startCh, toStopGet := kv.preGetStart(args, reply)

	// if already get the value with the key or not leader
	if toStopGet {
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

	// hear back from channel or timeout
	// https://stackoverflow.com/questions/49872097/idiomatic-way-for-reading-from-the-channel-for-a-certain-time
	select {
	case responseMsg := <-startCh.ReplyChannel:
		kv.handleGetResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		kv.getTimeout(op, reply)
		return
	}

}
