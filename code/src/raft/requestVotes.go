package raft

// struct

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// receiver side

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	DPrintf("EEE< node %v, term %v, status %v: self voted for %v, receive request vote from %v, term %v.\n",
		rf.me, rf.currentTerm, rf.status, rf.votedFor,
		args.CandidateId, args.Term)
	DPrintf("EEE< node %v, term %v, status %v: args.Term: %v, args.LastLogIndex: %v, rf.getLastLogEntryTerm(): %v, rf.getLastLogEntryIndex(): %v, rf.getLogLengthWithoutSnapshot(): %v, rf.snapshotEntriesAmount: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		args.Term, args.LastLogIndex,
		rf.getLastLogEntryTerm(), rf.getLastLogEntryIndex(), rf.getLogLengthWithoutSnapshot(), rf.snapshotEntriesAmount)
	DPrintf("EEE< node %v, term %v, status %v: rf.commitIndex: %v, rf.lastApplied: %v",
		rf.me, rf.currentTerm, rf.status,
		rf.commitIndex, rf.lastApplied)

	// TODO, in 2A, following requirement not implemented:
	// TODO, If votedFor is null or candidateId, and candidate’s log is at least as up-to-date as receiver’s log,
	// TODO, grant vote (§5.2, §5.4)
	// do not grant vote due to lower raft term
	if rf.currentTerm > args.Term {
		DPrintf("EEE< node %v, term %v, status %v: self term larger, reject vote.\n",
			rf.me, rf.currentTerm, rf.status)
		return
	}

	if rf.currentTerm < args.Term {
		DPrintf("EEE< node %v, term %v, status %v: self term lower, becomes follower of %v.\n",
			rf.me, rf.currentTerm, rf.status,
			args.CandidateId)
		rf.becomeFollower(args.Term, args.CandidateId)
	}

	DPrintf("EEE< node %v, term %v, status %v: rf.getLastLogEntryTerm(): %v, args.LastLogTerm: %v, rf.getLastLogEntryIndex(): %v, args.LastLogIndex: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.getLastLogEntryTerm(), args.LastLogTerm, rf.getLastLogEntryIndex(), args.LastLogIndex)

	// do not grand vote due to log not as up to date as this raft server
	//if (rf.getLastLogEntryTerm() > args.LastLogTerm) ||
	//	(rf.getLastLogEntryTerm() == args.LastLogTerm && rf.getLastLogEntryIndex() > args.LastLogIndex) {
	//	DPrintf("EEE< node %v, term %v, status %v: self log more up to date, reject vote.\n",
	//		rf.me, rf.currentTerm, rf.status)
	//	return
	//}
	if !rf.candidateIsUpToDate(args.LastLogTerm, args.LastLogIndex) {
		DPrintf("EEE< node %v, term %v, status %v: self log more up to date, reject vote.\n",
			rf.me, rf.currentTerm, rf.status)
		return
	} else {
		if rf.votedFor == rf.me && rf.status == Candidate {
			rf.becomeFollower(args.Term, args.CandidateId)
		}
	}

	if rf.hasntVote() {
		rf.grandOtherVote(args.CandidateId)
	}
	if rf.hasVotedFor(args.CandidateId) {
		rf.becomeFollower(args.Term, args.CandidateId)
		reply.VoteGranted = true
		reply.Term = rf.currentTerm
		DPrintf("EEE< node %v, term %v, status %v: vote granted, args.Term: %v, args.LastLogIndex: %v, rf.getLastLogEntryTerm(): %v, rf.getLastLogEntryIndex(): %v, rf.getLogLengthWithoutSnapshot(): %v, rf.getLastLogEntryIndex(): %v.\n",
			rf.me, rf.currentTerm, rf.status,
			args.Term, args.LastLogIndex,
			rf.getLastLogEntryTerm(), rf.getLastLogEntryIndex(), rf.getLogLengthWithoutSnapshot(), rf.getLastLogEntryIndex())
		DPrintf("EEE< node %v, term %v, status %v: rf.commitIndex: %v, rf.lastApplied: %v",
			rf.me, rf.currentTerm, rf.status,
			rf.commitIndex, rf.lastApplied)
	} else {
		DPrintf("EEE< node %v, term %v, status %v: refuse vote.\n", rf.me, rf.currentTerm, rf.status)
	}
}

// sender side

func (rf *Raft) checkElectionStatus() {
	DPrintf("EEE= node %v, term %v, status %v: check election status, number of vote gained: %v, total number of peers: %v.\n",
		rf.me, rf.currentTerm, rf.status,
		rf.numVoteGathered, len(rf.peers))

	if rf.status == Follower || rf.status == Leader {
		DPrintf("EEE= node %v, term %v, status %v: not candidate, quit election.\n",
			rf.me, rf.currentTerm, rf.status)

		return
	}
	if rf.numVoteGathered > (len(rf.peers) / 2) {
		rf.becomeLeader()
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	//DPrintf("EEE> node %v, term %v, status %v: send request vote to %v.\n",
	//	rf.me, rf.currentTerm, rf.status,
	//	server)

	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) handleSendRequestVote(server int) {
	// sendRequestVote() cannot be in critical section
	rf.mu.Lock()
	voteRequestArgs := rf.getRequestVoteArgs()
	voteRequestReply := RequestVoteReply{}
	rf.mu.Unlock()

	// TODO, RPC create in a new thread, would this ok be a promise like in network request, otherwise, why data race
	// TODO, do different goroutine share locks? what do they share exactly
	// TODO, are the lock the same lock for different threads?
	ok := rf.sendRequestVote(server, &voteRequestArgs, &voteRequestReply)
	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		// TODO, does print count as data race?
		DPrintf("EEE> node %v, term %v, status %v: from %v, term %v, voteGranted: %v.\n",
			rf.me, rf.currentTerm, rf.status,
			server, voteRequestArgs.Term, voteRequestReply.VoteGranted)
		DPrintf("EEE> node %v, term %v, status %v: rf.commitIndex: %v, rf.lastApplied: %v",
			rf.me, rf.currentTerm, rf.status,
			rf.commitIndex, rf.lastApplied)

		if voteRequestReply.Term > rf.currentTerm {
			DPrintf("EEE> node %v, term %v, status %v: higher term from %v, term %v, quit election and become follower.\n",
				rf.me, rf.currentTerm, rf.status,
				server, voteRequestArgs.Term)
			rf.becomeFollower(voteRequestReply.Term, -1)
			return
		}
		if voteRequestReply.VoteGranted == true {
			if voteRequestReply.Term == rf.currentTerm {
				DPrintf("EEE> node %v, term %v, status %v: vote gained from %v.\n",
					rf.me, rf.currentTerm, rf.status, server)
				rf.gainVote()
			} else {
				DPrintf("EEE> node %v, term %v, status %v: vote gained from %v but term is different voteRequestReply.Term: %v, rf.currentTerm: %v.\n",
					rf.me, rf.currentTerm, rf.status,
					server,
					voteRequestReply.Term, rf.currentTerm)
			}
		}

		rf.checkElectionStatus()
	} else {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		DPrintf("EEE> node %v, term %v, status %v: request vote to %v connection not success, not ok.\n",
			rf.me, rf.currentTerm, rf.status,
			server)
	}
}

func (rf *Raft) getRequestVoteArgs() RequestVoteArgs {
	voteRequestArg := RequestVoteArgs{}
	voteRequestArg.CandidateId = rf.me
	voteRequestArg.Term = rf.currentTerm
	voteRequestArg.LastLogTerm = rf.getLastLogEntryTerm()
	voteRequestArg.LastLogIndex = rf.getLastLogEntryIndex()
	return voteRequestArg
}

func (rf *Raft) candidateGatherVotes() {
	// construct a vote request arg that can be used for all request vote RPC for this server at this election

	for i, _ := range rf.peers {
		if i != rf.me {
			server := i
			DPrintf("EEE> node %v, term %v, status %v: request vote request to %v.\n",
				rf.me, rf.currentTerm, rf.status,
				server)

			go rf.handleSendRequestVote(server)
		}
	}
}
