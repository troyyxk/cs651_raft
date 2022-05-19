package shardkv

import (
	"6.824/raft"
	"log"
)

func (kv *ShardKV) handleInstallShardConfig(op Op) {
	DPrintf("coordinator server [%v], gid [%v], handle InstallShardConfig, config: %v, kv store: %v.\n",
		kv.me, kv.gid, op.ShardConfig, kv.state.ShardKVStore)

	newConfig := op.ShardConfig
	// if self is more up to date
	if newConfig.Num != kv.state.LatestConfig.Num+1 {
		return
	}
	// update config
	oldConfig := kv.state.LatestConfig
	kv.state.LatestConfig = newConfig
	kv.state.AllConfigs = append(kv.state.AllConfigs, newConfig)

	// if first config
	if kv.state.LatestConfig.Num == 1 {
		for i, curGId := range kv.state.LatestConfig.Shards {
			if curGId == kv.gid {
				kv.state.ShardTimestamp[i] = kv.state.LatestConfig.Num
			} else {
				kv.state.ShardTimestamp[i] = 0
			}
			// add empty map to the kv db
			kv.state.ShardKVStore[i] = createEmptyKeyValueStore()
		}
		DPrintf("coordinator server [%v], gid [%v], request index [%v], handle InstallShardConfig, first time, current shard timestamp: %v, op config's shards arrangement: %v, kv store: %v.\n",
			kv.me, kv.gid, op.RequestIndex, kv.state.ShardTimestamp, op.ShardConfig.Shards, kv.state.ShardKVStore)
		return
	}

	// for all the rest, update timestamp for shards continue to own
	for i, curGId := range kv.state.LatestConfig.Shards {
		if curGId == kv.gid && oldConfig.Shards[i] == kv.gid {
			kv.state.ShardTimestamp[i] = kv.state.LatestConfig.Num
		}
	}
	DPrintf("coordinator server [%v], gid [%v], request index [%v], handle InstallShardConfig, current shard timestamp: %v, op config's shards arrangement: %v, kv store: %v.\n",
		kv.me, kv.gid, op.RequestIndex, kv.state.ShardTimestamp, op.ShardConfig.Shards, kv.state.ShardKVStore)
}

func (kv *ShardKV) handleAcquireShard(op Op) {
	// TODO, add check
	DPrintf("coordinator server [%v], gid [%v], request index [%v], handle AcquireShard, shard id [%v], local vs op timestamp: %v/%v, local vs op kv store: %v/%v.\n",
		kv.me, kv.gid, op.RequestIndex, op.ShardId, kv.state.ShardTimestamp[op.ShardId], op.ShardConfigNum, kv.state.ShardKVStore[op.ShardId], op.ShardData)
	if op.ShardConfigNum > kv.state.ShardTimestamp[op.ShardId] {
		kv.state.ShardTimestamp[op.ShardId] = op.ShardConfigNum
		kv.state.ShardKVStore[op.ShardId] = op.ShardData.clone()
	}
	// take in the client last request as well
	for cliendId, clientLastRequestIndex := range op.ClientLastRequest {
		if kv.state.ClientLastRequest[cliendId] < clientLastRequestIndex {
			kv.state.ClientLastRequest[cliendId] = clientLastRequestIndex
		}
	}
}

func (kv *ShardKV) handleCommandApply(m raft.ApplyMsg) {
	op := m.Command.(Op)
	replyMsg := KVReplyMsg{}
	replyMsg.Error = ErrFiller

	if !op.NeedSendBackCh {
		// either config or acquire shard
		if op.Type == InstallShardConfig {
			kv.handleInstallShardConfig(op)
		} else if op.Type == AcquireShard {
			kv.handleAcquireShard(op)
		} else {
			log.Fatalf("In handleCommandApply(), op.Type: %v, not in [InstallShardConfig, AcquireShard]\n", op.Type)
		}
	} else {

		// check if is for this group
		targetShard := key2shard(op.Key)
		targetGroup := kv.state.LatestConfig.Shards[targetShard]
		if kv.gid != targetGroup {
			replyMsg.Error = ErrWrongGroup
		} else if _, allUpTODate := kv.allShardsUpToDate(); !allUpTODate {
			// check if config up to date
			replyMsg.Error = ErrNotUpToDate
		} else if kv.isDuplicateOp(op) {
			DPrintf("duplicated, gid[%v], server [%v], duplicate op, op.ClientId: %v, op.RequestIndex: %v, kv.state.clientLastRequest: %v.\n",
				kv.gid, kv.me, op.ClientId, op.RequestIndex, kv.state.ClientLastRequest)
			// for PUT, APPEND and GET default for key exist
			replyMsg.Error = OK
			// for the GET
			if op.Type == GET {
				replyMsg.Value, replyMsg.Error = kv.getValue(op)
			}
		} else {
			kv.state.ClientLastRequest[op.ClientId] = op.RequestIndex
			var resultValue string
			if op.Type == GET {
				replyMsg.Value, replyMsg.Error = kv.getValue(op)
				DPrintf("coordinator server [%v], gid [%v], client id [%v], request index [%v], shard index [%v], get, key: %v, get result value: %v.\n",
					kv.me, kv.gid, op.ClientId, op.RequestIndex, targetShard, op.Key, replyMsg.Value)
			} else if op.Type == PUT {
				kv.setKeyValue(op.Key, op.Value)
				if resultValue, _ = kv.getValueWithKey(op.Key); resultValue != op.Value {
					log.Fatalf("In apply ticker, set value does not work!")
				}
				DPrintf("coordinator server [%v], gid [%v], client id [%v], request index [%v], shard index [%v], put, key: %v, value: %v, result after put: %v.\n",
					kv.me, kv.gid, op.ClientId, op.RequestIndex, targetShard, op.Key, op.Value, resultValue)
				replyMsg.Error = OK
			} else if op.Type == APPEND {
				if val, ok := kv.getValueWithKey(op.Key); ok {
					kv.setKeyValue(op.Key, val+op.Value)
					if resultValue, _ = kv.getValueWithKey(op.Key); resultValue != val+op.Value {
						log.Fatalf("In apply ticker, set value does not work!")
					}
				} else {
					kv.setKeyValue(op.Key, op.Value)
					if resultValue, _ = kv.getValueWithKey(op.Key); resultValue != op.Value {
						log.Fatalf("In apply ticker, set value does not work!")
					}
				}
				DPrintf("coordinator server [%v], gid [%v], client id [%v], request index [%v], shard index [%v], APPEND, key: %v, value: %v, result after append: %v..\n",
					kv.me, kv.gid, op.ClientId, op.RequestIndex, targetShard, op.Key, op.Value, resultValue)
				replyMsg.Error = OK
			} else {
				log.Fatalf("In handleCommandApply(), op.Type: %v, op.Type not in [GET, PUT, APPEND]\n", op.Type)
			}
		}
	}

	// if overflow, set count back to 0, and snapshot
	// op.NeedSendBackCh == false means a configuration change or getting or sending a shard, therefore need to snapshot
	// immediately
	if kv.maxraftstate != -1 && (op.NeedSendBackCh == false || kv.rf.GetPersister().RaftStateSize() >= kv.maxraftstate) {
		snapshot := kv.makeSnapshot()
		kv.rf.Snapshot(m.CommandIndex, snapshot)
	}

	if _, isLeader := kv.rf.GetState(); !isLeader {
		replyMsg.Error = ErrWrongLeader
	}

	if op.NeedSendBackCh && kv.opChannelMatch(m) {
		kv.signalStartCh(m, replyMsg)
	}
}

func (kv *ShardKV) opChannelMatch(m raft.ApplyMsg) bool {
	responseOp := m.Command.(Op)

	if responseOp.ServerId != kv.me {
		return false
	}

	startCh, ok := kv.getStartCh(responseOp)
	if !ok {
		return false
	}
	originalOp := startCh.Operation

	if originalOp.Key != responseOp.Key ||
		originalOp.Value != responseOp.Value ||
		originalOp.Type != responseOp.Type ||
		originalOp.ClientId != responseOp.ClientId ||
		originalOp.RequestIndex != responseOp.RequestIndex {
		DPrintf("In opChannelMatch(), key, value or type of the original op and response op does not match.")
		return false
	}

	return true
}

func (kv *ShardKV) signalStartCh(m raft.ApplyMsg, replyMsg KVReplyMsg) {
	responseOp := m.Command.(Op)
	if !(responseOp.Type == GET ||
		responseOp.Type == PUT ||
		responseOp.Type == APPEND) {
		log.Fatalln("In signalStartCh, operation type not in [GET, PUT, APPEND].")
	}
	startCh, ok := kv.getStartCh(responseOp)
	if !ok {
		DPrintf("coordinator, gid [%v], server [%v], channel does not exist\n",
			kv.gid, kv.me)
		return
	}
	DPrintf("coordinator, gid [%v], server [%v], send\n",
		kv.gid, kv.me)
	startCh.ReplyChannel <- replyMsg
	kv.deleteStartCh(responseOp)
	DPrintf("coordinator, gid [%v], server [%v], finish send\n",
		kv.gid, kv.me)
}

func (kv *ShardKV) handleApplyMessage(m raft.ApplyMsg) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	// because the first one is a filler log, need this duplicated code
	//if kv.totalLogCount == 0 {
	//	kv.instoreLogCount++
	//	kv.totalLogCount++
	//	return
	//}

	if m.SnapshotValid {
		if kv.rf.CondInstallSnapshot(m.SnapshotTerm, m.SnapshotIndex, m.Snapshot) {
			kv.installSnapshot(m.Snapshot)
		}
	} else if m.CommandValid {
		kv.handleCommandApply(m)
	} else {
		if m.CommandIndex != 0 {
			panic("not snapshot, not command, and index not 0")
		}
	}

}

func (kv *ShardKV) applyTicker() {
	DPrintf("STT= ShardKV: %v, gid: %v, start applyCh ticker\n",
		kv.me, kv.gid)

	for {
		select {
		case m, ok := <-kv.applyCh:
			{
				if !ok {
					DPrintf("In applyTicker(), apply channel not ok.\n")
					return
				}
				kv.handleApplyMessage(m)
			}
		}
	}
}
