package raft

import (
	"math/rand"
	"time"
)

func getTimeoutValue() int {
	// 151 here as we want a number 150-300 and rand.Intn(n) returns an int [0, n)
	// as mentioned in hint, timeout need to be larger, so times 3 would be reasonable to make 10 heart beats per second
	// (1 heart beat per 100ms) reasonable

	return 150 + rand.Intn(151)
}

func getTimeoutDuration() time.Duration {
	return time.Duration(getTimeoutValue()) * time.Millisecond
}

func getNewTimeoutTime() time.Time {
	return time.Now().Add(getTimeoutDuration())
}

func (rf *Raft) setNewElectionTime() {
	rf.nextElectionTimeout = getNewTimeoutTime()
}

func (rf *Raft) grandOtherVote(voteForId int) {
	DPrintf("EEE< node %v, term %v, status %v: vote granted to %v.\n", rf.me, rf.currentTerm, rf.status, voteForId)

	rf.votedFor = voteForId
}

func (rf *Raft) hasntVote() bool {
	return rf.hasVotedFor(-1)
}

// -1 means has not voted yet
func (rf *Raft) hasVotedFor(candidateId int) bool {
	return rf.votedFor == candidateId
}

func (rf *Raft) isLeader() bool {
	return rf.status == Leader
}

func (rf *Raft) becomeLeader() {
	// become leader
	rf.status = Leader
	rf.currentTerm += 1
	rf.votedFor = -1
	rf.numVoteGathered = 0
	rf.nextElectionTimeout = getNewTimeoutTime()

	// leader metadata of the followers
	rf.nextIndex = make([]int, len(rf.peers))
	for i, _ := range rf.peers {
		rf.nextIndex[i] = rf.getLastLogEntryIndex() + 1
	}
	rf.matchIndex = make([]int, len(rf.peers))
	for i, _ := range rf.peers {
		rf.matchIndex[i] = -1
	}

	DPrintf("EEE= node %v, term %v, status %v: becomes leader.\n",
		rf.me, rf.currentTerm, rf.status)
	rf.broadcastAppendEntries()
	rf.persist()
}

func (rf *Raft) becomeCandidate() {
	rf.currentTerm += 1
	rf.status = Candidate
	rf.votedFor = rf.me
	rf.numVoteGathered = 1
	rf.persist()
}

func (rf *Raft) becomeFollower(newTerm int, votedForId int) {
	rf.currentTerm = newTerm
	rf.votedFor = votedForId

	rf.status = Follower
	rf.numVoteGathered = 0
	rf.nextElectionTimeout = getNewTimeoutTime()
	rf.persist()
}

func (rf *Raft) gainVote() {
	if rf.status == Candidate {
		rf.numVoteGathered += 1
	}
}
