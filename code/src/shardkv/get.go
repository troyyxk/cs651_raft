package shardkv

import (
	"log"
	"time"
)

// +++ client side +++

func (ck *Clerk) createGetArgs(key string) GetArgs {
	args := GetArgs{}
	args.Key = key
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++
	return args
}

//
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
// You will have to modify this function.
//
func (ck *Clerk) Get(key string) string {
	args := ck.createGetArgs(key)

	for {
		shard := key2shard(key)
		gid := ck.config.Shards[shard]
		if servers, ok := ck.config.Groups[gid]; ok {
			DPrintf("client [%v], RequestIndex [%v], start get request, shard [%v], to gid [%v], key: %v\n",
				ck.clerkRandomId, args.RequestIndex, shard, gid, args.Key)
			// try each server for the shard.
			for si := 0; si < len(servers); si++ {
				srv := ck.make_end(servers[si])
				DPrintf("client [%v], RequestIndex [%v], get request, shard [%v], to gid [%v], to server: %v\n",
					ck.clerkRandomId, args.RequestIndex, shard, gid, srv)
				var reply GetReply
				ok := srv.Call("ShardKV.Get", &args, &reply)
				if ok && (reply.Err == OK || reply.Err == ErrNoKey) {
					return reply.Value
				}
				if ok && (reply.Err == ErrWrongGroup) {
					break
				}
				// ... not ok, or ErrWrongLeader
			}
		}
		time.Sleep(100 * time.Millisecond)
		// ask controler for the latest configuration.
		ck.config = ck.sm.Query(-1)
	}

	return ""
}

// +++ server side +++

func (kv *ShardKV) createGetOp(args *GetArgs) Op {
	op := Op{}
	op.ServerId = kv.me

	op.Type = GET
	op.Key = args.Key

	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex

	op.NeedSendBackCh = true
	return op
}

// return op and whether to stop
func (kv *ShardKV) preGetStart(args *GetArgs, reply *GetReply) (Op, OpKVReplyChannel, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], get, key: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, args.RequestIndex, args.Key, kv.state.ShardKVStore)

	var fillerOp Op
	var fillerChannel OpKVReplyChannel

	// have not receive config, do not accept
	if kv.state.LatestConfig.Num == 0 {
		DPrintf("gid [%v], server [%v], get, no config.\n",
			kv.gid, kv.me)
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
		DPrintf("gid [%v], server [%v], get, config not up to date, config: %v, timestamp: %v.\n",
			kv.gid, kv.me, kv.state.LatestConfig, kv.state.ShardTimestamp)
		reply.Err = ErrNotUpToDate
		return fillerOp, fillerChannel, true
	}

	// create the op for this get
	op := kv.createGetOp(args)

	DPrintf("gid [%v], server [%v], get start raft, key: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, args.Key, kv.state.ShardKVStore)

	startCh := kv.createStartCh(op)
	return op, startCh, false
}

func (kv *ShardKV) handleGetResult(op Op, responseMsg KVReplyMsg, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], get, return error type: %v, key: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, op.RequestIndex, responseMsg.Error, op.Key, kv.state.ShardKVStore)

	kv.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleGetResult(), response error is ErrFiller, which should not be")
	}

	reply.Err, reply.Value = responseMsg.Error, responseMsg.Value
}

func (kv *ShardKV) getTimeout(op Op, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	DPrintf("gid [%v], server [%v], RequestIndex [%v], get, timeout, key: %v, ShardKVStore: %v.\n",
		kv.gid, kv.me, op.RequestIndex, op.Key, kv.state.ShardKVStore)

	kv.deleteStartCh(op)

	reply.Err = ErrTimeout
}

func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) {
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

	op, startCh, toStopGet := kv.preGetStart(args, reply)

	// if already get the value with the key or not leader
	if toStopGet {
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

	// hear back from channel or timeout
	select {
	case responseMsg := <-startCh.ReplyChannel:
		kv.handleGetResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		kv.getTimeout(op, reply)
		return
	}

}
