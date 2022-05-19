package raft

import (
	"log"
)

func (rf *Raft) leaderUpdateCommit() {
	DPrintf("CCC= node %v, term %v, status %v: try update leader commit with rf.commitIndex: %v, rf.getLogLengthWithoutSnapshot(): %v, rf.getLastLogEntryIndex(): %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.commitIndex, rf.getLogLengthWithoutSnapshot(), rf.getLastLogEntryIndex())
	DPrintf("CCC= node %v, term %v, status %v: rf.matchIndex: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.matchIndex)
	newCommitIndex := rf.commitIndex
	// from the first in the log to the last in the log
	for j := 0; j < rf.getLogLengthWithoutSnapshot(); j++ {
		// from the first in the log to the last in the log
		i := rf.getIndexFromOffsetIndex(j)
		// the committed index do not go back
		if i < newCommitIndex {
			continue
		}
		// cont if this log is written to majority, if written, set as new commit
		count := 1
		for j := 0; j < len(rf.peers); j++ {
			if j != rf.me && rf.matchIndex[j] >= i {
				count++
			}
		}

		// rule 1, commit at majority
		// rule 2, 5.4.2, only commit the log and the log before when it is at the current term
		if count > len(rf.peers)/2 && rf.getLogEntryAtIndex(i).Term == rf.currentTerm {
			DPrintf("CCC= node %v, term %v, status %v: update newCommitIndex from %v to %v, count: %v, len(rf.peers)/2: %v, rf.getLogEntryAtIndex(i).Term: %v, rf.currentTerm: %v.\n",
				rf.me, rf.currentTerm, rf.status,
				newCommitIndex, i,
				count, len(rf.peers)/2, rf.getLogEntryAtIndex(i).Term, rf.currentTerm)
			newCommitIndex = i
		}
	}
	if newCommitIndex < rf.commitIndex {
		log.Fatalln("In commitAndApplyUtils.go, new commit smaller than previous commit")
	}
	if newCommitIndex > -1 && rf.commitIndex != newCommitIndex && rf.getLogEntryAtIndex(newCommitIndex).Term != rf.currentTerm {
		log.Fatalln("In commitAndApplyUtils.go, against 5.4.2, not committing the ones at current term")
	}
	rf.commitIndex = newCommitIndex
	DPrintf("CCC= node %v, term %v, status %v: finish update leader commit with rf.commitIndex: %v, len(rf.logs): %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.commitIndex, len(rf.logs))

}

func (rf *Raft) commitToApplyChannelWithLock() []ApplyMsg {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	applyMsgs := make([]ApplyMsg, 0)
	rf.persist()
	// if no change, just return
	DPrintf("CCC= node %v, term %v, status %v: try commit to apply channel with rf.lastApplied: %v, rf.commitIndex: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.lastApplied, rf.commitIndex)

	if rf.lastApplied+1 < rf.snapshotEntriesAmount {
		DPrintf("CCC= node %v, term %v, status %v: snapshot ahead, commit snapshot, rf.lastApplied: %v, rf.snapshotEntriesAmount: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			rf.lastApplied, rf.snapshotEntriesAmount)

		applyMsg := ApplyMsg{}
		applyMsg.CommandValid = false
		applyMsg.SnapshotValid = true
		applyMsg.Snapshot = rf.persister.ReadSnapshot()
		applyMsg.SnapshotTerm = rf.lastSnapshotTerm
		applyMsg.SnapshotIndex = rf.snapshotEntriesAmount - 1

		rf.lastApplied = rf.snapshotEntriesAmount - 1

		applyMsgs = append(applyMsgs, applyMsg)
		return applyMsgs
	}

	if rf.lastApplied >= rf.commitIndex {
		return applyMsgs
	}

	//applied := false
	// send new message to apply channel
	for i := rf.lastApplied + 1; i < rf.commitIndex+1; i++ {
		// todo, do i need separate lock for it
		applyMsg := ApplyMsg{}
		applyMsg.CommandValid = true
		applyMsg.Command = rf.getLogEntryAtIndex(i).Action
		applyMsg.CommandIndex = i

		if applyMsg.CommandIndex == 0 {
			applyMsg.CommandValid = false
		}

		//applied = true
		DPrintf("CCC= node %v, term %v, status %v: try to apply with applyMsg.Command: %v, applyMsg.CommandIndex: %v, rf.getLogEntryAtIndex(i).Term: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			applyMsg.Command, applyMsg.CommandIndex, rf.getLogEntryAtIndex(i).Term)

		//rf.applyCh <- applyMsg
		applyMsgs = append(applyMsgs, applyMsg)

	}
	//if applied && rf.getLogEntryAtIndex(rf.commitIndex).Term != rf.currentTerm {
	//	log.Fatalf("In commitAndApplyUtils.go, not commiting the current term\n")
	//}

	// update lastApplied to commitIndex after applying them
	rf.lastApplied = rf.commitIndex
	DPrintf("CCC= node %v, term %v, status %v: apply succeed.\n",
		rf.me, rf.currentTerm, rf.status)

	return applyMsgs

}

func (rf *Raft) commitToApplyChannel() {
	rf.applyMu.Lock()
	defer rf.applyMu.Unlock()

	applyMsgs := rf.commitToApplyChannelWithLock()
	for _, msg := range applyMsgs {
		DPrintf("CCC= commit message: %v.\n",
			msg)
		DPrintf("msg: %v\n", msg)
		rf.applyCh <- msg
	}
	DPrintf("CCC= finish commit.\n")

}
