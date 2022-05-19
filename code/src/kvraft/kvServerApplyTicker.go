package kvraft

import (
	"6.824/raft"
	"log"
)

func (kv *KVServer) handleCommandApply(m raft.ApplyMsg) {
	op := m.Command.(Op)
	replyMsg := KVReplyMsg{}
	replyMsg.Error = ErrFiller

	if kv.isDuplicateOp(op) {
		DPrintf("STT< kvserver: %v, duplicate op, op.ClientId: %v, op.RequestIndex: %v, kv.state.clientLastRequest: %v.\n",
			kv.me, op.ClientId, op.RequestIndex, kv.state.ClientLastRequest)
		// for PUT, APPEND and GET default for key exist
		replyMsg.Error = OK
		// for the GET
		if op.Type == GET {
			replyMsg.Value, replyMsg.Error = kv.getValue(op)
		}
	} else {
		kv.state.ClientLastRequest[op.ClientId] = op.RequestIndex
		if op.Type == GET {
			replyMsg.Value, replyMsg.Error = kv.getValue(op)
		} else if op.Type == PUT {
			kv.state.KeyValueStore[op.Key] = op.Value
			replyMsg.Error = OK
		} else if op.Type == APPEND {
			if val, ok := kv.state.KeyValueStore[op.Key]; ok {
				kv.state.KeyValueStore[op.Key] = val + op.Value
			} else {
				kv.state.KeyValueStore[op.Key] = op.Value
			}
			replyMsg.Error = OK
		} else {
			log.Fatalln("In handleCommandApply(), op.Type not in [GET, PUT, APPEND]")
		}
	}

	// if overflow, set count back to 0, and snapshot
	if kv.maxraftstate != -1 && kv.rf.GetPersister().RaftStateSize() >= kv.maxraftstate {
		snapshot := kv.makeSnapshot()
		kv.rf.Snapshot(m.CommandIndex, snapshot)
	}

	if _, isLeader := kv.rf.GetState(); !isLeader {
		replyMsg.Error = ErrWrongLeader
	}

	if kv.opChannelMatch(m) {
		DPrintf("STT> kvserver: %v, before sending to apply channel, op.OpIndex: %v, kv.startChannels: %v.\n",
			kv.me, op.OpIndex, kv.startChannels)
		kv.signalStartCh(m, replyMsg)
	}
}

func (kv *KVServer) opChannelMatch(m raft.ApplyMsg) bool {
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

func (kv *KVServer) signalStartCh(m raft.ApplyMsg, replyMsg KVReplyMsg) {
	responseOp := m.Command.(Op)
	startCh, ok := kv.getStartCh(responseOp)
	if !ok {
		DPrintf("STT> kvserver: %v, channel for CommandIndex: %v does not exist\n",
			kv.me, m.CommandIndex)
		return
	}
	DPrintf("STT> kvserver: %v, send\n",
		kv.me)
	startCh.ReplyChannel <- replyMsg
	kv.deleteStartCh(responseOp)
	DPrintf("STT> kvserver: %v, finish send\n",
		kv.me)
}

func (kv *KVServer) handleApplyMessage(m raft.ApplyMsg) {
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
		//fmt.Println(m)
		if m.CommandIndex != 0 {
			panic("not snapshot, not command, and index not 0")
		}
	}

}

func (kv *KVServer) applyTicker() {
	DPrintf("STT= kvserver: %v, start applyCh ticker\n",
		kv.me)

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
