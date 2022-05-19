package shardkv

import (
	"6.824/shardctrler"
	"log"
	"time"
)

// +++ client side ++

func (kv *ShardKV) createAcquireShards(sid int, targetConfig shardctrler.Config) AcquireShardArgs {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	args := AcquireShardArgs{}
	args.Gid = kv.gid
	args.ConfigNum = targetConfig.Num
	args.ShardIndex = sid
	return args
}

func (kv *ShardKV) createAcquireShardOp(sid int, args AcquireShardArgs, reply AcquireShardReply) Op {
	op := Op{}
	op.ServerId = kv.me

	op.Type = AcquireShard

	op.NeedSendBackCh = false
	op.ShardId = sid
	op.ShardConfigNum = reply.ConfigNum
	op.ShardData = reply.ShardData
	op.ClientLastRequest = reply.ClientLastRequest

	return op
}

func (kv *ShardKV) handleAcquireShardReply(sid int, args AcquireShardArgs, reply AcquireShardReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if _, isLeader := kv.rf.GetState(); !isLeader {
		DPrintf("gid [%v], server [%v], not leader, do not start acquire shard op.\n",
			kv.gid, kv.me)
		return
	}
	//if reply.ShardTimestamp <= kv.state.ShardTimestamp[sid] {
	//	return
	//}
	op := kv.createAcquireShardOp(sid, args, reply)
	kv.rf.Start(op)
}

func (kv *ShardKV) acquireShards(sid int, targetConfig shardctrler.Config) {
	DPrintf("gid [%v], server [%v], acquire shard with sid [%v].\n",
		kv.gid, kv.me, sid)
	args := kv.createAcquireShards(sid, targetConfig)
	gid := targetConfig.Shards[sid]
	servers := targetConfig.Groups[gid]
	for si := 0; si < len(servers); si++ {

		srv := kv.make_end(servers[si])
		var reply AcquireShardReply
		ok := srv.Call("ShardKV.AcquireShards", &args, &reply)
		if ok {
			if reply.Error == ASFiller {
				log.Fatalln("In acquireShards(), reply.Error is ASFiller, shall not happen")
			}
			if reply.Error == ASOK {
				DPrintf("gid [%v], server [%v], acquireShards ok, sid [%v], reply config num: %v, reply timestamp: %v, cur time stamp: %v.\n",
					kv.gid, kv.me, sid, reply.ConfigNum, reply.ShardTimestamp, kv.state.LatestConfig.Num)
				kv.handleAcquireShardReply(sid, args, reply)
				return
			}
		}
	}
	DPrintf("gid [%v], server [%v], ERROR!!!, acquireShards non work, sid [%v].\n",
		kv.gid, kv.me, sid)
}

// +++ server side +++

func (kv *ShardKV) AcquireShards(args *AcquireShardArgs, reply *AcquireShardReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	reply.Error = ASFiller

	DPrintf("gid [%v], server [%v],received AcquireShards from gid [%v], config num [%v], shard index[%v].\n",
		kv.gid, kv.me, args.Gid, args.ConfigNum, args.ShardIndex)

	if _, isLeader := kv.rf.GetState(); !isLeader {
		DPrintf("gid [%v], server [%v], WRONG LEADER, received AcquireShards from gid [%v], config num [%v], shard index[%v].\n",
			kv.gid, kv.me, args.Gid, args.ConfigNum, args.ShardIndex)
		reply.Error = ASWrongLeader
		return
	}

	if kv.state.LatestConfig.Num == 0 || kv.state.LatestConfig.Num < args.ConfigNum {
		DPrintf("gid [%v], server [%v], LATEST SHARD NUM IS 0, received AcquireShards from gid [%v], config num [%v], shard index[%v].\n",
			kv.gid, kv.me, args.Gid, args.ConfigNum, args.ShardIndex)
		reply.Error = ASBehind
		return
	}

	// if self timestamp for the shard with sid is equal or smaller than the requesting one, report error
	//if shardTimestamp, ok := kv.state.ShardTimestamp[args.ShardIndex]; ok {
	//	if shardTimestamp <= args.ConfigNum {
	//		DPrintf("gid [%v], server [%v], BEHIND, target shard id: %v, args timestamp: %v, local timestamp: %v.\n",
	//			kv.gid, kv.me, args.ShardIndex, args.ConfigNum, shardTimestamp)
	//		reply.Error = ASBehind
	//		return
	//	}
	//} else {
	//	log.Fatalln("In AcquireShards(), shard with no assigned timestamp.")
	//}

	reply.ConfigNum = kv.state.LatestConfig.Num
	reply.ShardTimestamp = kv.state.ShardTimestamp[args.ShardIndex]
	reply.Error = ASOK
	var targetKV KeyValueStore
	targetKV = kv.state.ShardKVStore[args.ShardIndex]
	reply.ShardData = targetKV.clone()
	reply.ClientLastRequest = clientLastRequestClone(kv.state.ClientLastRequest)
	DPrintf("gid [%v], server [%v], acquire shard OK, args config num [%v], reply config num [%v], shard timestamp [%v], reply/og data: %v vs %v.\n",
		kv.gid, kv.me, args.ConfigNum, reply.ConfigNum, reply.ShardTimestamp, reply.ShardData, kv.state.ShardKVStore[args.ShardIndex])
}

// +++ ticker +++

func (kv *ShardKV) acquireShardTicker() {
	DPrintf("server [%v], gid [%v], start acquire shard ticker\n",
		kv.me, kv.gid)
	for {
		select {
		case <-time.After(AcquireShardWaitTime * time.Millisecond):
			if _, isLeader := kv.rf.GetState(); !isLeader {
				continue
			}
			// leader, try to see if there are lagging shard
			laggingShards, allShardsSynced := kv.allShardsUpToDateLocked()
			if !allShardsSynced {
				DPrintf("gid [%v], server [%v], in acquireShardTicker(), lagging shards:%v, config: %v, timestamp: %v, kv store: %v.\n",
					kv.gid, kv.me, laggingShards, kv.state.LatestConfig, kv.state.ShardTimestamp, kv.state.ShardKVStore)
				// get the previous config, the second last one
				targetConfig := kv.state.AllConfigs[len(kv.state.AllConfigs)-2]
				for _, sid := range laggingShards {
					go kv.acquireShards(sid, targetConfig)
				}
			}

		}
	}
}
