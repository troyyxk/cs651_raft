package shardctrler

import (
	"log"
	"time"
)

//  +++ client side +++

func (ck *Clerk) Leave(gids []int) {
	args := &LeaveArgs{}
	// Your code here.

	// operation info
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++

	// request argument parameters
	args.GIDs = gids

	for {
		// try each known server.
		for i, srv := range ck.servers {
			DPrintf("CK[%v]: Client side Move, to server index at: %v, RequestIndex: %v, GIDs: %v\n",
				ck.clerkRandomId, i, args.RequestIndex, args.GIDs)
			var reply LeaveReply
			ok := srv.Call("ShardCtrler.Leave", args, &reply)
			if ok && reply.WrongLeader == false {
				if reply.Err != OK {
					log.Fatalln("In client request side, wrong leader is true but err is not wrong leader bug.")
				}
				return
			}
			DPrintf("CK[%v]: result error: %v, Client side Move, to server index at: %v, RequestIndex: %v, GIDs: %v\n",
				ck.clerkRandomId, reply.Err, i, args.RequestIndex, args.GIDs)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// +++ server side +++

func (sc *ShardCtrler) creatLeaveOp(args *LeaveArgs) Op {
	op := Op{}
	op.ServerId = sc.me

	op.Type = LEAVE

	// parameter
	op.GIDs = args.GIDs

	// identifier
	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

func (sc *ShardCtrler) preLeaveStart(args *LeaveArgs, reply *LeaveReply) (Op, OpKVReplyChannel, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, trying to leave.\n",
		sc.me)

	// create the op for this get
	op := sc.creatLeaveOp(args)
	startCh := sc.createStartCh(op)

	return op, startCh, false
}

func (sc *ShardCtrler) handleLeaveResult(op Op, responseMsg ReplyMsg, reply *LeaveReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, op: %v, responseMsg: %v\n",
		sc.me, op, responseMsg)

	sc.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleLeaveResult(), response error is ErrFiller, which should not be")
	}

	reply.Err = responseMsg.Error
	if reply.Err != OK {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
	}
}

func (sc *ShardCtrler) leaveTimeout(op Op, reply *LeaveReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		sc.me, op.ClientId, op.OpIndex)

	sc.deleteStartCh(op)

	reply.Err, reply.WrongLeader = ErrTimeout, true
	return
}

func (sc *ShardCtrler) Leave(args *LeaveArgs, reply *LeaveReply) {
	// Your code here.
	if _, isLeader := sc.rf.GetState(); !isLeader {
		reply.Err, reply.WrongLeader = ErrWrongLeader, true
		return
	}

	DPrintf("SC[%v]: Join, server listener side, RequestIndex: %v, GIDs: %v, ClientId: %v\n",
		sc.me, args.RequestIndex, args.GIDs, args.ClientId)

	op, startCh, _ := sc.preLeaveStart(args, reply)

	// set error first
	reply.Err, reply.WrongLeader = ErrOther, true

	//sc.rf.Start(op)
	_, _, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.Err, reply.WrongLeader = ErrWrongLeader, true
		sc.deleteStartChLocked(op)
		return
	}

	DPrintf("SPP> is leader, send op.")

	select {
	case responseMsg := <-startCh.ReplyChannel:
		sc.handleLeaveResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		sc.leaveTimeout(op, reply)
		return
	}
}
