package shardctrler

import (
	"log"

	"6.824/raft"
)

func (sc *ShardCtrler) handleJoin(servers map[int][]string) {
	newConfig := sc.makeNextConfig()

	// add the new groups, if there are duplication in id, report error
	for k, v := range servers {
		//if _, ok := newConfig.Groups[k]; ok {
		//	log.Fatalln("In handleJoin(), there are duplicate id")
		//}
		newConfig.Groups[k] = v
	}

	// fill the void
	assignVoidShards(&newConfig)

	// redistrubute
	if len(newConfig.Groups) > 1 {
		redistributeShards(&newConfig)
	}

	// add the new config to the list of configs
	sc.state.Configs = append(sc.state.Configs, newConfig)
}

func (sc *ShardCtrler) handleLeave(GIDs []int) {
	newConfig := sc.makeNextConfig()

	// delete the group
	for _, gid := range GIDs {
		delete(newConfig.Groups, gid)
	}

	// remove the groups with the GIDs
	for i, gid := range newConfig.Shards {
		if contains(GIDs, gid) {
			newConfig.Shards[i] = 0
		}
	}

	// fill the void
	assignVoidShards(&newConfig)

	// redistrubute
	if len(newConfig.Groups) > 1 {
		redistributeShards(&newConfig)
	}

	// add the new config to the list of configs
	sc.state.Configs = append(sc.state.Configs, newConfig)
}

func (sc *ShardCtrler) handleMove(Shard int, GID int) {
	newConfig := sc.makeNextConfig()

	newConfig.Shards[Shard] = GID

	// add the new config to the list of configs
	sc.state.Configs = append(sc.state.Configs, newConfig)
}

func (sc *ShardCtrler) handleCommandApply(m raft.ApplyMsg) {
	op := m.Command.(Op)
	replyMsg := ReplyMsg{}
	replyMsg.Error = ErrFiller

	DPrintf("SC[%v]: "+
		"server ticker side, receive an operation, Type: %v, RequestIndex: %v, ServerId: %v, ClientId: %v\n",
		sc.me, op.Type, op.RequestIndex, op.ServerId, op.ClientId)

	if sc.isDuplicateOp(op) {
		DPrintf("STT< kvserver: %v, duplicate op, op.ClientId: %v, op.RequestIndex: %v, kv.state.clientLastRequest: %v.\n",
			sc.me, op.ClientId, op.RequestIndex, sc.state.ClientLastRequest)
		replyMsg.Error = OK
		if op.Type == QUERY {
			replyMsg.ReturnConfig = sc.getValue(op)
			replyMsg.Error = OK
		}
	} else {
		sc.state.ClientLastRequest[op.ClientId] = op.RequestIndex
		if op.Type == QUERY {
			replyMsg.ReturnConfig = sc.getValue(op)
			replyMsg.Error = OK
		} else if op.Type == JOIN {
			sc.handleJoin(op.Servers)
			replyMsg.Error = OK
		} else if op.Type == LEAVE {
			sc.handleLeave(op.GIDs)
			replyMsg.Error = OK
		} else if op.Type == MOVE {
			sc.handleMove(op.Shard, op.GID)
			replyMsg.Error = OK
		} else {
			log.Fatalln("In handleCommandApply(), op.Type not in [JOIN, LEAVE, MOVE, QUERY]")
		}
	}

	// make snapshot
	snapshot := sc.makeSnapshot()
	sc.rf.Snapshot(m.CommandIndex, snapshot)

	if _, isLeader := sc.rf.GetState(); !isLeader {
		replyMsg.Error = ErrWrongLeader
		DPrintf("SC[%v]: server ticker side, not leader\n",
			sc.me)
	}

	if sc.opChannelMatch(m) {
		DPrintf("STT> kvserver: %v, before sending to apply channel, op.OpIndex: %v, kv.startChannels: %v.\n",
			sc.me, op.OpIndex, sc.startChannels)
		sc.signalStartCh(m, replyMsg)
	}
}

func (sc *ShardCtrler) handleApplyMessage(m raft.ApplyMsg) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if m.SnapshotValid {
		if sc.rf.CondInstallSnapshot(m.SnapshotTerm, m.SnapshotIndex, m.Snapshot) {
			//sc.installSnapshot(m.Snapshot)
		}
	} else if m.CommandValid {
		sc.handleCommandApply(m)
	} else {
		//fmt.Println(m)
		if m.CommandIndex != 0 {
			panic("not snapshot, not command, and index not 0")
		}
	}

}

func (sc *ShardCtrler) applyTicker() {
	DPrintf("STT= kvserver: %v, start applyCh ticker\n",
		sc.me)

	for {
		select {
		case m, ok := <-sc.applyCh:
			{
				if !ok {
					DPrintf("In applyTicker(), apply channel not ok.\n")
					return
				}
				sc.handleApplyMessage(m)
			}
		}
	}
}
