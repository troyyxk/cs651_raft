package raft

// structs

type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
	// Offset            int
	// Done              bool
}

type InstallSnapshotReply struct {
	Term int
}

// commmunicate with state machine

//
// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
//
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {
	//rf.mu.Lock()
	//defer rf.mu.Unlock()
	//
	//DPrintf("SSS> node %v, term %v, status %v: conditional install snapshot from state machine, lastIncludedTerm: %v, lastIncludedIndex: %v.\n",
	//	rf.me, rf.currentTerm, rf.status,
	//	lastIncludedTerm, lastIncludedIndex)
	//
	//// Your code here (2D).
	//// todo, how do you define more recent here
	////if rf.snapshotEntriesAmount-1 > lastIncludedIndex {
	//if rf.lastApplied > lastIncludedIndex {
	//	DPrintf("SSS> node %v, term %v, status %v: conditional install snapshot not success, rf.snapshotEntriesAmount: %v, lastIncludedIndex: %v.\n",
	//		rf.me, rf.currentTerm, rf.status,
	//		rf.snapshotEntriesAmount, lastIncludedIndex)
	//	return false
	//}
	//
	//DPrintf("SSS> node %v, term %v, status %v: conditional install snapshot success.\n",
	//	rf.me, rf.currentTerm, rf.status)
	//
	//rf.snapshotEntriesAmount = lastIncludedIndex + 1
	//rf.lastSnapshotTerm = lastIncludedTerm
	//rf.persistAndSnapshot(snapshot)

	return true
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// Your code here (2D).

	DPrintf("SSS> node %v, term %v, status %v: order from state machine to install snapshot at index: %v, len(rf.logs): %v, rf.snapshotEntriesAmount: %v, rf.logs: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		index, len(rf.logs), rf.snapshotEntriesAmount, rf.logs)

	if rf.snapshotEntriesAmount-1 >= index || index > rf.commitIndex {
		DPrintf("SSS> node %v, term %v, status %v: order from state machine to install snapshot not success, index: %v, rf.snapshotEntriesAmount: %v, rf.commitIndex: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			index, rf.snapshotEntriesAmount, rf.commitIndex)
		return
	}

	DPrintf("SSS> node %v, term %v, status %v: order from state machine to install snapshot success, index: %v, rf.snapshotEntriesAmount: %v, rf.commitIndex: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		index, rf.snapshotEntriesAmount, rf.commitIndex)

	rf.lastSnapshotTerm = rf.getLogEntryAtIndex(index).Term
	rf.logs = rf.getLogEntryStartingAt(index + 1)
	rf.snapshotEntriesAmount = index + 1
	if index > rf.lastApplied {
		rf.lastApplied = index
	}
	//if index < rf.lastApplied {
	//	log.Fatalf("SSS> node %v, term %v, status %v: index <= rf.lastApplied, index: %v, rf.lastApplied: %v.\n",
	//		rf.me, rf.currentTerm, rf.status,
	//		index, rf.lastApplied)
	//}

	DPrintf("SSS> node %v, term %v, status %v: finish Snapshot() with rf.commitIndex: %v,  len(rf.logs): %v, rf.snapshotEntriesAmount: %v, rf.logs: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.commitIndex,
		len(rf.logs), rf.snapshotEntriesAmount, rf.logs)

	rf.persistAndSnapshot(snapshot)
}

// receiver side

func (rf *Raft) InstallSnapshotLocked(args *InstallSnapshotArgs, reply *InstallSnapshotReply) (bool, ApplyMsg) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	applyMsg := ApplyMsg{}

	DPrintf("SSS< node %v, term %v, status %v: install snapshot from node %v, term %v.\n",
		rf.me, rf.currentTerm, rf.status,
		args.LeaderId, args.Term)

	if args.Term < rf.currentTerm {
		DPrintf("SSS< node %v, term %v, status %v: term smaller, return .\n",
			rf.me, rf.currentTerm, rf.status)
		reply.Term = rf.currentTerm
		return false, applyMsg
	}

	// if the term match, become follower
	rf.becomeFollower(args.Term, -1)
	rf.persist()

	reply.Term = rf.currentTerm

	// if args last snapshot index smaller or equal, return
	if rf.snapshotEntriesAmount > args.LastIncludedIndex {
		DPrintf("SSS< node %v, term %v, status %v: args.LastIncludedIndex already included, return, rf.snapshotEntriesAmount: %v, args.LastIncludedIndex: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			rf.snapshotEntriesAmount, args.LastIncludedIndex)
		return false, applyMsg
	}

	// update local snapshot information and log
	// todo, which one go first
	rf.lastSnapshotTerm = args.LastIncludedTerm
	rf.logs = rf.getLogEntryStartingAt(args.LastIncludedIndex + 1)
	rf.snapshotEntriesAmount = args.LastIncludedIndex + 1

	//if args.LastIncludedIndex > rf.commitIndex {
	//	rf.commitIndex = args.LastIncludedIndex
	//}
	//if args.LastIncludedIndex > rf.lastApplied {
	//	rf.lastApplied = args.LastIncludedIndex
	//}

	rf.persistAndSnapshot(args.Data)

	applyMsg.CommandValid = false
	applyMsg.SnapshotValid = true
	applyMsg.Snapshot = args.Data
	applyMsg.SnapshotTerm = rf.lastSnapshotTerm
	applyMsg.SnapshotIndex = rf.snapshotEntriesAmount - 1
	//fmt.Printf("args.LastIncludedIndex: %v, rf.lastApplied: %v, rf.snapshotEntriesAmount: %v, applyMsg.SnapshotIndex: %v\n",
	//	args.LastIncludedIndex, rf.lastApplied, rf.snapshotEntriesAmount, applyMsg.SnapshotIndex)

	return true, applyMsg
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.applyMu.Lock()
	defer rf.applyMu.Unlock()

	ok, _ := rf.InstallSnapshotLocked(args, reply)
	if ok {
		//DPrintf("server %v: InstallSnapshot< applyMsg: %v\n", rf.me, applyMsg)
		//rf.applyCh <- applyMsg
		//DPrintf("server %v: InstallSnapshot< finish\n", rf.me)
	}
}

// sender side

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

func (rf *Raft) createInstallSnapshotArgs() InstallSnapshotArgs {
	installSnapshotArg := InstallSnapshotArgs{}
	installSnapshotArg.Term = rf.currentTerm
	installSnapshotArg.LeaderId = rf.me

	installSnapshotArg.LastIncludedIndex = rf.snapshotEntriesAmount - 1
	installSnapshotArg.LastIncludedTerm = rf.lastSnapshotTerm

	installSnapshotArg.Data = rf.persister.ReadSnapshot()

	return installSnapshotArg
}

func (rf *Raft) handleSendInstallSnapshot(server int) {
	rf.mu.Lock()
	args := rf.createInstallSnapshotArgs()
	reply := InstallSnapshotReply{}
	DPrintf("SSS> node %v, term %v, status %v: send install snapshot to %v, rf.nextIndex: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		server, rf.nextIndex)
	rf.mu.Unlock()

	ok := rf.sendInstallSnapshot(server, &args, &reply)

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		if rf.currentTerm < reply.Term {
			DPrintf("SSS> node %v, term %v, status %v: send install snapshot to %v, term is bigger, become follower to with term %v.\n",
				rf.me, rf.currentTerm, rf.status,
				server, reply.Term)
			rf.becomeFollower(reply.Term, -1)
		}

	} else {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		DPrintf("SSS> node %v, term %v, status %v: send snap shot to %v connection not success, not ok.\n",
			rf.me, rf.currentTerm, rf.status,
			server)

		rf.nextIndex[server] = args.LastIncludedIndex + 1
		rf.matchIndex[server] = args.LastIncludedIndex

	}
}
