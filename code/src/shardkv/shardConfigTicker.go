package shardkv

import (
	"6.824/shardctrler"
	"time"
)

func (kv *ShardKV) createInstallShardConfigOp(shardConfig shardctrler.Config) Op {
	op := Op{}
	op.ServerId = kv.me

	op.Type = InstallShardConfig

	op.NeedSendBackCh = false
	op.ShardConfig = shardConfig
	return op
}

func (kv *ShardKV) shardConfigTicker() {
	DPrintf("STT= ShardKV: %v, gid: %v, start shard configuration ticker\n",
		kv.me, kv.gid)

	for {
		select {
		case <-time.After(ShardConfigWaitTime * time.Millisecond):
			if _, isLeader := kv.rf.GetState(); !isLeader {
				continue
			}
			// if not all up to date, do not get new config
			if _, allUpToDate := kv.allShardsUpToDateLocked(); !allUpToDate {
				continue
			}
			// leader, try to get new config
			newConfig := kv.mck.Query(kv.state.LatestConfig.Num + 1)
			if newConfig.Num != kv.state.LatestConfig.Num+1 {
				continue
			}
			DPrintf("gid [%v], server [%v], shard config Ticker, new config: %v, cur config: %v.\n",
				kv.gid, kv.me, newConfig, kv.state.LatestConfig)
			//// install new configuration
			//kv.state.LatestConfig = newConfig
			// distribute new configuration when get new configuration
			installShardConfigOp := kv.createInstallShardConfigOp(newConfig)
			if _, isLeader := kv.rf.GetState(); isLeader {
				kv.rf.Start(installShardConfigOp)
			}
		}
	}
}
