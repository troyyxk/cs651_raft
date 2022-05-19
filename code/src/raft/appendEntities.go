package raft

// struct

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntity
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term           int
	Success        bool
	InSnapshot     bool
	SnapshotAmount int
}

// receiver side

func (rf *Raft) AppendEntriesWithLock(args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	DPrintf("AAA< node %v, term %v, status %v: append entries from node %v, term %v, args.PrevLogIndex: %v, args.PrevLogTerm: %v; self: rf.snapshotEntriesAmount: %v, rf.lastSnapshotTerm: %v, len(rf.logs): %v, len(args.Entries),: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		args.LeaderId, args.Term, args.PrevLogIndex, args.PrevLogTerm,
		rf.snapshotEntriesAmount, rf.lastSnapshotTerm, len(rf.logs), len(args.Entries))

	if rf.currentTerm > args.Term {
		DPrintf("AAA< node %v, term %v, status %v: term smaller, refused.\n",
			rf.me, rf.currentTerm, rf.status)
		reply.Success = false
		reply.Term = rf.currentTerm
		return false
	}

	// if the term match, become follower
	rf.becomeFollower(args.Term, -1)
	rf.persist()
	// if the appended entries start in snapshot
	if args.PrevLogIndex < rf.snapshotEntriesAmount-1 {
		DPrintf("AAA< node %v, term %v, status %v: append entry in the snapshot, send back stored amount info.\n",
			rf.me, rf.currentTerm, rf.status)
		reply.Success = false
		reply.Term = rf.currentTerm
		reply.InSnapshot = true
		reply.SnapshotAmount = rf.snapshotEntriesAmount
		return false
	}
	// if prev log index larger than expected, return false
	if args.PrevLogIndex > rf.getLastLogEntryIndex() {
		DPrintf("AAA< node %v, term %v, status %v: prev log index: %v, out of range of rf.getLastLogEntryIndex(): %v.\n",
			rf.me, rf.currentTerm, rf.status,
			args.PrevLogIndex, rf.getLastLogEntryIndex())
		reply.Success = false
		reply.Term = rf.currentTerm
		return false
	}

	// check if matches, if not matches, return false
	if !rf.logMatches(args.PrevLogTerm, args.PrevLogIndex) {
		DPrintf("AAA< node %v, term %v, status %v: logs do not match, args.PrevLogTerm: %v, args.PrevLogIndex: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			args.PrevLogTerm, args.PrevLogIndex)
		reply.Success = false
		reply.Term = rf.currentTerm
		return false
	}

	// at this point, all should match, remove redundant and start appending
	rf.removeLogFromIndex(args.PrevLogIndex)
	rf.appendEntries(args.Entries)

	reply.Success = true
	reply.Term = rf.currentTerm

	// if leader commit advance, start commit here as well
	if rf.commitIndex < args.LeaderCommit {
		DPrintf("AAA< node %v, term %v, status %v: commit index change from %v to %v.\n",
			rf.me, rf.currentTerm, rf.status,
			rf.commitIndex, args.LeaderCommit)
		rf.commitIndex = args.LeaderCommit
	}
	rf.persist()

	DPrintf("AAA< node %v, term %v, status %v: successfully append entries, last log term: %v, last log index %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.getLastLogEntryTerm(), rf.getLastLogEntryIndex())

	return true
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	ok := rf.AppendEntriesWithLock(args, reply)
	if ok {
		rf.commitToApplyChannel()
	}
}

// sender side

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	//DPrintf("AAA> node %v, term %v, status %v: send heart beat to %v.\n",
	//	rf.me, rf.currentTerm, rf.status,
	//	server)

	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) handleSendAppendEntries(server int) {
	rf.mu.Lock()

	// might committed, check to see if need to send snapshot
	if rf.nextIndex[server] < rf.snapshotEntriesAmount {
		rf.nextIndex[server] = rf.snapshotEntriesAmount
		go rf.handleSendInstallSnapshot(server)
		rf.mu.Unlock()
		return
	}

	// if not leader, do not send append entry
	if rf.status != Leader {
		DPrintf("AAA> node %v, term %v, status %v: not leader, stop sending, tried send append entries to %v.\n",
			rf.me, rf.currentTerm, rf.status,
			server)
		rf.mu.Unlock()
		return
	}

	// no needs for snapshot now, send normal append entries
	args := rf.createAppendEntriesArgs(server)
	reply := AppendEntriesReply{}
	DPrintf("AAA> node %v, term %v, status %v: start send append entries to %v, rf.nextIndex: %v; self: rf.snapshotEntriesAmount: %v, rf.lastSnapshotTerm: %v, len(rf.logs): %v.\n",
		rf.me, rf.currentTerm, rf.status,
		server, rf.nextIndex,
		rf.snapshotEntriesAmount, rf.lastSnapshotTerm, len(rf.logs))
	rf.mu.Unlock()

	ok := rf.sendAppendEntries(server, &args, &reply)
	// todo, implement apply channel part
	if ok {
		rf.mu.Lock()
		//defer rf.mu.Unlock()
		DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v.\n",
			rf.me, rf.currentTerm, rf.status,
			server)

		// todo, might be teh bug
		if rf.status != Leader {
			DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v, no longer leader, stop sending append entries.\n",
				rf.me, rf.currentTerm, rf.status,
				server)
			rf.mu.Unlock()
		} else if reply.Success {
			DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v, success.\n",
				rf.me, rf.currentTerm, rf.status,
				server)
			// success, updated rf.nextIndex, rf.matchIndex
			// todo update commitIndex and lastApplied while apply channel
			rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
			rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
			rf.leaderUpdateCommit()
			rf.mu.Unlock()
			rf.commitToApplyChannel()
			//rf.mu.Unlock()
		} else if reply.InSnapshot {
			DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v, replay indicate in snapshot.\n",
				rf.me, rf.currentTerm, rf.status,
				server)
			rf.nextIndex[server] = reply.SnapshotAmount
			rf.mu.Unlock()
		} else if rf.currentTerm < reply.Term {
			DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v, term is bigger, become follower to with term %v.\n",
				rf.me, rf.currentTerm, rf.status,
				server, reply.Term)
			rf.becomeFollower(reply.Term, -1)
			rf.mu.Unlock()
		} else {
			DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v, not success, args.PrevLogIndex: %v, args.PrevLogTerm: %v, "+
				"rf.nextIndex[server]: %v, rf.snapshotEntriesAmount: %v, rf.lastSnapshotTerm: %v.\n",
				rf.me, rf.currentTerm, rf.status,
				server, args.PrevLogIndex, args.PrevLogTerm, rf.nextIndex[server],
				rf.snapshotEntriesAmount, rf.lastSnapshotTerm)
			rf.nextIndex[server] -= 1

			// todo, check which one
			//if rf.nextIndex[server] < rf.snapshotEntriesAmount-1 {
			//	rf.nextIndex[server] = rf.snapshotEntriesAmount - 1
			//}
			if rf.nextIndex[server] < rf.snapshotEntriesAmount {
				rf.nextIndex[server] = rf.snapshotEntriesAmount
			}
			// for non-snapshot, shall not go below 1
			if rf.nextIndex[server] < 1 {
				rf.nextIndex[server] = 1
			}

			preCurIndex := rf.nextIndex[server] - 1
			curTerm := rf.getLogTermAtIndex(preCurIndex)
			// stop at == 1 or reach a new term
			for preCurIndex > 1 && preCurIndex > rf.snapshotEntriesAmount && rf.getLogEntryAtIndex(preCurIndex).Term == curTerm {
				preCurIndex -= 1
			}
			rf.nextIndex[server] = preCurIndex + 1

			// at the lower limit of the current log in raft memory, send snapshot
			// also under the condition that the leader has any snapshot to send
			if rf.nextIndex[server] <= rf.snapshotEntriesAmount && rf.snapshotEntriesAmount > 0 {
				go rf.handleSendInstallSnapshot(server)
			}

			//appendEntriesArg := rf.createAppendEntriesArgs(server)
			//appendEntriesReply := AppendEntriesReply{}
			//
			//DPrintf("AAA> node %v, term %v, status %v: RESEND!!! append entries send to %v with appendEntriesArg.PrevLogIndex: %v, appendEntriesArg.PrevLogTerm: %v.\n",
			//	rf.me, rf.currentTerm, rf.status,
			//	server, appendEntriesArg.PrevLogIndex, appendEntriesArg.PrevLogTerm)
			//
			//go rf.handleSendAppendEntries(server, &appendEntriesArg, &appendEntriesReply)
			rf.mu.Unlock()
		}
	} else {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		DPrintf("AAA> node %v, term %v, status %v: receive send append entries to %v connection not success, not ok.\n",
			rf.me, rf.currentTerm, rf.status,
			server)
	}
}

func (rf *Raft) createAppendEntriesArgs(server int) AppendEntriesArgs {
	appendEntriesArg := AppendEntriesArgs{}
	appendEntriesArg.Term = rf.currentTerm
	appendEntriesArg.LeaderId = rf.me

	appendEntriesArg.PrevLogIndex = rf.getLogIndexBeforeIndex(rf.nextIndex[server])
	appendEntriesArg.PrevLogTerm = rf.getLogTermAtIndex(appendEntriesArg.PrevLogIndex)

	appendEntriesArg.Entries = rf.getLogEntryStartingAt(appendEntriesArg.PrevLogIndex + 1)
	appendEntriesArg.LeaderCommit = rf.commitIndex
	return appendEntriesArg
}

func (rf *Raft) broadcastAppendEntries() {
	DPrintf("AAA> node %v, term %v, status %v: start sending out heart beat.\n",
		rf.me, rf.currentTerm, rf.status)

	for i, _ := range rf.peers {
		if i != rf.me {
			server := i

			DPrintf("AAA> node %v, term %v, status %v: append entries send to %v.\n",
				rf.me, rf.currentTerm, rf.status,
				server)

			go rf.handleSendAppendEntries(server)
		}
	}
}
