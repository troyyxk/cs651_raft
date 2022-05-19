package shardkv

import (
	"log"
	"time"
)

// +++ client side +++

func (ck *Clerk) createPutAppendArgs(key string, value string, op OpType) PutAppendArgs {
	args := PutAppendArgs{}
	args.Key = key
	args.Value = value
	args.Op = op
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++
	return args
}

//
// shared by Put and Append.
// You will have to modify this function.
//
func (ck *Clerk) PutAppend(key string, value string, op OpType) {
	args := ck.createPutAppendArgs(key, value, op)

	for {
		shard := key2shard(key)
		gid := ck.config.Shards[shard]
		if servers, ok := ck.config.Groups[gid]; ok {
			DPrintf("client [%v], RequestIndex [%v], start %v request, shard [%v], to gid [%v], key: %v, value: %v\n",
				ck.clerkRandomId, args.RequestIndex, op, shard, gid, args.Key, args.Value)
			for si := 0; si < len(servers); si++ {
				srv := ck.make_end(servers[si])
				DPrintf("client [%v], RequestIndex [%v], %v, to shard [%v], gid [%v], request to server %v\n",
					ck.clerkRandomId, args.RequestIndex, op, shard, gid, srv)
				var reply PutAppendReply
				ok := srv.Call("ShardKV.PutAppend", &args, &reply)
				if ok && reply.Err == OK {
					return
				}
				if ok && reply.Err == ErrWrongGroup {
					break
				}
				// ... not ok, or ErrWrongLeader
			}
		}
		time.Sleep(100 * time.Millisecond)
		// ask controler for the latest configuration.
		ck.config = ck.sm.Query(-1)
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, PUT)
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, APPEND)
}

// +++ server side +++

func (kv *ShardKV) createPutAppendOp(args *PutAppendArgs) Op {
	op := Op{}
	op.ServerId = kv.me

	op.Type = args.Op
	op.Key = args.Key
	op.Value = args.Value

	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex

	op.NeedSendBackCh = true
	return op
}

// return op and whether to stop
func (kv *ShardKV) prePutAppendStart(args *PutAppendArgs, reply *PutAppendReply) (Op, OpKVReplyChannel, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], %v, key: %v, value: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, args.RequestIndex, args.Op, args.Key, args.Value, kv.state.ShardKVStore)

	var fillerOp Op
	var fillerChannel OpKVReplyChannel

	// have not receive config, do not accept
	if kv.state.LatestConfig.Num == 0 {
		DPrintf("gid [%v], server [%v], %v, no config.\n",
			kv.gid, kv.me, args.Op)
		reply.Err = ErrNoConfig
		return fillerOp, fillerChannel, true
	}

	//if kv.mck.Query(-1).Num != kv.state.LatestConfig.Num {
	//	reply.Err = ErrUnmatchedConfig
	//	return fillerOp, fillerChannel, true
	//}

	// check if correct group
	targetShard := key2shard(args.Key)
	targetGroup := kv.state.LatestConfig.Shards[targetShard]
	if kv.gid != targetGroup {
		reply.Err = ErrWrongGroup
		return fillerOp, fillerChannel, true
	}

	if _, allUpTODate := kv.allShardsUpToDate(); !allUpTODate {
		DPrintf("gid [%v], server [%v], %v, config not up to date, config: %v, timestamp: %v.\n",
			kv.gid, kv.me, args.Op, kv.state.LatestConfig, kv.state.ShardTimestamp)
		reply.Err = ErrNotUpToDate
		return fillerOp, fillerChannel, true
	}

	// create the op for this get
	op := kv.createPutAppendOp(args)

	DPrintf("gid [%v], server [%v], RequestIndex [%v], %v start raft, key: %v, value: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, args.RequestIndex, args.Op, args.Key, args.Value, kv.state.ShardKVStore)

	startCh := kv.createStartCh(op)
	return op, startCh, false
}

func (kv *ShardKV) handlePutAppendResult(op Op, responseMsg KVReplyMsg, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], %v, return error type: %v, key: %v, value: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, op.RequestIndex, op.Type, responseMsg.Error, op.Key, op.Value, kv.state.ShardKVStore)

	kv.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handlePutAppendResult(), response error is ErrFiller, which should not be")
	}

	reply.Err = responseMsg.Error
}

func (kv *ShardKV) putAppendTimeout(op Op, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], %v, timeout, key: %v, value: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, op.RequestIndex, op.Type, op.Key, op.Value, kv.state.ShardKVStore)

	kv.deleteStartCh(op)

	reply.Err = ErrTimeout
}

func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// set error first
	reply.Err = ErrFiller

	// Your code here.
	// if not leader return
	if _, isLeader := kv.rf.GetState(); !isLeader {
		reply.Err = ErrWrongLeader
		return
	}

	//if !kv.containShardLocked(args.Key) {
	//	reply.Err = ErrWrongGroup
	//	return
	//}

	op, startCh, toStopPutAppend := kv.prePutAppendStart(args, reply)

	// if already get the value with the key or not leader
	if toStopPutAppend {
		return
	}

	//kv.rf.Start(op)
	_, _, isLeader := kv.rf.Start(op)
	if !isLeader {
		reply.Err = ErrWrongLeader
		DPrintf("gid [%v], server [%v], not leader\n",
			kv.gid, kv.me)
		kv.deleteStartChLocked(op)
		return
	}

	select {
	case responseMsg := <-startCh.ReplyChannel:
		kv.handlePutAppendResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		kv.putAppendTimeout(op, reply)
		return
	}
}
