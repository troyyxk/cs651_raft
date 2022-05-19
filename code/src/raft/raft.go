package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"6.824/labgob"
	"bytes"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"6.824/labrpc"
)

// import "bytes"
// import "6.824/labgob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type LogEntity struct {
	Term   int
	Action interface{}
}

type RaftStatus int

const (
	Follower  RaftStatus = 0
	Candidate RaftStatus = 1
	Leader    RaftStatus = 2
)

// timing
const (
	HeartBeatInterval int = 25
)

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// apply channel
	applyCh chan ApplyMsg
	applyMu sync.Mutex

	// status attributes
	status              RaftStatus
	numVoteGathered     int
	nextElectionTimeout time.Time

	// Persistent state on all servers:
	currentTerm int
	votedFor    int
	logs        []LogEntity

	// snapshot
	snapshotEntriesAmount int
	lastSnapshotTerm      int

	// Volatile state on all servers:
	// todo, what is the use of lastApplied and matchIndex
	// todo, do all nodes needs to commit, submit to applyCh, or jus the leader
	commitIndex int
	lastApplied int

	// Volatile state on leaders:
	// (Reinitialized after election)
	nextIndex  []int
	matchIndex []int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	term = rf.currentTerm
	isleader = rf.status == Leader

	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//

func (rf *Raft) getPersistData() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.logs)
	e.Encode(rf.snapshotEntriesAmount)
	e.Encode(rf.lastSnapshotTerm)
	data := w.Bytes()
	return data
}

func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	data := rf.getPersistData()
	rf.persister.SaveRaftState(data)
}

func (rf *Raft) persistAndSnapshot(snapshot []byte) {
	data := rf.getPersistData()
	rf.persister.SaveStateAndSnapshot(data, snapshot)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var persistCurrentTrem int
	var persistVoteFor int
	var persistLogs []LogEntity
	var persistSnapshotEntriesAmount int
	var persistLastSnapshotTerm int

	if d.Decode(&persistCurrentTrem) != nil ||
		d.Decode(&persistVoteFor) != nil ||
		d.Decode(&persistLogs) != nil ||
		d.Decode(&persistSnapshotEntriesAmount) != nil ||
		d.Decode(&persistLastSnapshotTerm) != nil {
		log.Fatalln("read persist error")
	} else {
		rf.currentTerm = persistCurrentTrem
		rf.votedFor = persistVoteFor
		rf.logs = persistLogs
		rf.snapshotEntriesAmount = persistSnapshotEntriesAmount
		rf.lastSnapshotTerm = persistLastSnapshotTerm
	}
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// todo, does the follower need to send the command to the leader?

	// todo, what does it mean by "otherwise start the agreement and return immediately"
	// todo, what is a state machine
	// todo, how many logs do wen send, all of them at once? as the leader does not know the status of log on worker
	// todo, how do newly elected leader know the committed index

	// gold
	// https://www.youtube.com/watch?v=vYp4LYbnnW8
	// design choice, send recursive as in the video or send all at once.

	index := rf.getLastLogEntryIndex() + 1
	term := rf.currentTerm
	isLeader := rf.status == Leader

	// return if not leader
	if rf.status != Leader {
		return index, term, isLeader
	}

	// leader section, add the new log entry to log entries, send out append entries
	// send to apply channel is in broadcastAppendEntries->handleSendAppendEntries
	rf.appendToLogEntries(command)
	//rf.broadcastAppendEntries()
	rf.persist()

	return index, term, isLeader

}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	for rf.killed() == false {

		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().
		rf.mu.Lock()
		if rf.isLeader() {
			rf.broadcastAppendEntries()
		} else {
			if time.Now().After(rf.nextElectionTimeout) {
				rf.setNewElectionTime()
				rf.startElection()
			}
		}
		rf.mu.Unlock()

		// 10 heart beats per second == 1 heart beat per millisecond
		time.Sleep(time.Duration(HeartBeatInterval) * time.Millisecond)

	}
}

//func (rf *Raft) applyTicker() {
//	for rf.killed() == false {
//
//		// Your code here to check if a leader election should
//		// be started and to randomize sleeping time using
//		// time.Sleep().
//		rf.commitToApplyChannel()
//
//		// 10 heart beats per second == 1 heart beat per millisecond
//		time.Sleep(time.Duration(HeartBeatInterval) * time.Millisecond)
//
//	}
//}

func (rf *Raft) startElection() {
	DPrintf("---= node %v, term %v, status %v: start election.\n", rf.me, rf.currentTerm, rf.status)

	rf.becomeCandidate()
	DPrintf("---= node %v, term %v, status %v: become candidate and start gather vote.\n", rf.me, rf.currentTerm, rf.status)

	rf.candidateGatherVotes()
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.applyCh = applyCh

	// Your initialization code here (2A, 2B, 2C).
	// start as a follower
	rf.status = Follower
	// start at term 0 and continue to grow
	rf.currentTerm = 0
	// have not decided who to vote, voteFor = -1, if decided, votedFor = the candidate index; reset back to -1 after
	// election
	rf.votedFor = -1
	rf.numVoteGathered = 0
	rf.nextElectionTimeout = getNewTimeoutTime()

	rf.logs = make([]LogEntity, 1)
	rf.logs[0] = LogEntity{-1, -1}

	rf.commitIndex = -1
	rf.lastApplied = -1

	rf.snapshotEntriesAmount = 0
	rf.lastSnapshotTerm = -1

	DPrintf("--- node %v, term %v, status %v: Initialized index %v raft node.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.me)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	DPrintf("PPP= node %v, term %v, status %v: after read persist, self: rf.currentTerm: %v, rf.votedFor: %v, "+
		"len(rf.logs): %v, rf.snapshotEntriesAmount: %v, rf.lastSnapshotTerm: %v, rf.logs: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.currentTerm, rf.votedFor, len(rf.logs), rf.snapshotEntriesAmount, rf.lastSnapshotTerm, rf.logs)

	// start ticker goroutine to start elections
	go rf.ticker()
	//go rf.applyTicker()

	//if rf.snapshotEntriesAmount > 0 {
	//	rf.lastApplied = rf.snapshotEntriesAmount - 1
	//	applyMsg := ApplyMsg{}
	//	applyMsg.CommandValid = false
	//	applyMsg.SnapshotValid = true
	//	applyMsg.Snapshot = persister.ReadSnapshot()
	//	applyMsg.SnapshotTerm = rf.lastSnapshotTerm
	//	applyMsg.SnapshotIndex = rf.snapshotEntriesAmount - 1
	//	rf.applyCh <- applyMsg
	//}

	return rf
}
