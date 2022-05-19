package raft

import (
	"fmt"
	"log"
)

// REMAINDER, all index here refer to the actual index that will ben committed and applied
// offset index refer to the index

// REMAINDER, there should be no rf.logs outside this file except raft.go for initializing and persist

func (rf *Raft) getLastLogEntryTerm() int {
	if len(rf.logs) == 0 {
		return rf.lastSnapshotTerm
	}
	return rf.getLogEntryAtIndex(rf.getLastLogEntryIndex()).Term
}

func (rf *Raft) getLastLogEntryIndex() int {
	return len(rf.logs) - 1 + rf.snapshotEntriesAmount
}

func (rf *Raft) getLogFullLength() int {
	return len(rf.logs) + rf.snapshotEntriesAmount
}

// return the length of the log without snapshot
func (rf *Raft) getLogLengthWithoutSnapshot() int {
	return len(rf.logs)
}

func (rf *Raft) logIndexCheck(index int, funcName string, lowerBoundCheck, upperBoundCheck bool) {
	//DPrintf("LLL= node %v, term %v, status %v: in logIndexCheck() called by %v().\n",
	//	rf.me, rf.currentTerm, rf.status,
	//	funcName)
	if lowerBoundCheck && index < rf.snapshotEntriesAmount {
		log.Fatalf("In logUtils.go, node: %v, logIndexCheck(), called by %v, trying to access index in snapshot, index: %v, len(rf.logs): %v, rf.snapshotEntriesAmount: %v.\n",
			rf.me,
			funcName, index, len(rf.logs),
			rf.snapshotEntriesAmount)
	}
	if upperBoundCheck && index > rf.getLastLogEntryIndex() {
		log.Fatalf("In logUtils.go, logIndexCheck(), called by %v, trying to access index in bigger than log, index: %v, len(rf.logs): %v, rf.snapshotEntriesAmount: %v.\n", funcName, index, len(rf.logs), rf.snapshotEntriesAmount)
	}
}

func (rf *Raft) getLogIndexBeforeIndex(index int) int {
	//rf.logIndexCheck(index, "getLogIndexBeforeIndex", true, true)
	if index == -1 {
		return -1
	}
	return index - 1
}

func (rf *Raft) getLogTermAtIndex(index int) int {
	if index+1 == rf.snapshotEntriesAmount {
		return rf.lastSnapshotTerm
	}
	rf.logIndexCheck(index, "getLogTermAtIndex", true, true)
	return rf.getLogEntryAtIndex(index).Term
}

func (rf *Raft) getIndexFromOffsetIndex(offsetIndex int) int {
	index := offsetIndex + rf.snapshotEntriesAmount
	rf.logIndexCheck(index, "getIndexFromOffsetIndex", true, true)
	return index
}

func (rf *Raft) getOffsetIndex(index int) int {
	// rf.logIndexCheck(index, "getOffsetIndex", true, true)
	return index - rf.snapshotEntriesAmount
}

func (rf *Raft) getLogEntryAtIndex(index int) LogEntity {
	rf.logIndexCheck(index, "getLogEntryAtIndex", true, true)
	if index == -1 {
		log.Fatalf("In logUtils.go, trying to access index at -1.\n")

		//var emptyInterface interface{}
		//return LogEntity{-1, emptyInterface}
	}
	return rf.logs[rf.getOffsetIndex(index)]
}

func (rf *Raft) candidateIsUpToDate(candidateLastTerm, candidateLastIndex int) bool {
	if candidateLastTerm > rf.getLastLogEntryTerm() {
		return true
	}
	// todo, should it be > there?
	if candidateLastTerm == rf.getLastLogEntryTerm() && candidateLastIndex >= rf.getLastLogEntryIndex() {
		return true
	}
	return false
}

func (rf *Raft) removeLogFromIndex(index int) {
	// todo, if this condition is necessary
	// if the index, prev index in append entries rpc, is in snap shot, do nothing
	if index+1 != rf.snapshotEntriesAmount {
		rf.logIndexCheck(index, "removeLogFromIndex", true, true)
	}
	if index == -1 {
		log.Fatalf("In logUtils.go, removeLogFromIndex(), trying to access index at -1.\n")
		//fmt.Printf("In logUtils.go, remove all")
		//rf.logs = make([]LogEntity, 0)
		//// todo, remove the //
		//return
	}
	rf.logs = rf.logs[:rf.getOffsetIndex(index)+1]
}

func (rf *Raft) logMatches(term, index int) bool {
	rf.logIndexCheck(index, "logMatches", false, true)
	// if start empty, start true
	if index == -1 {
		return true
	}
	if index > rf.getLastLogEntryIndex() {
		return false
	}
	if rf.getLogTermAtIndex(index) != term {
		return false
	}
	return true
}

func (rf *Raft) copyStaringIndex(index int) {
	rf.logIndexCheck(index, "copyStaringIndex", true, true)
	if index == -1 {
		fmt.Printf("In logUtils.go, copy all")
		rf.logs = make([]LogEntity, 0)
	}
	rf.logs = rf.logs[:rf.getOffsetIndex(index)+1]
}

func (rf *Raft) appendToLogEntries(command interface{}) {
	rf.logs = append(rf.logs, LogEntity{rf.currentTerm, command})
}

func (rf *Raft) getLogEntryStartingAt(index int) []LogEntity {
	rf.logIndexCheck(index, "getLogEntryStartingAt", true, false)
	copiedLogs := make([]LogEntity, 0)
	if rf.getOffsetIndex(index)+1 > len(rf.logs) {
		return copiedLogs
	}
	copiedLogs = append(copiedLogs, rf.logs[rf.getOffsetIndex(index):]...)
	return copiedLogs
}

func (rf *Raft) appendEntries(entries []LogEntity) {
	rf.logs = append(rf.logs, entries...)
}
