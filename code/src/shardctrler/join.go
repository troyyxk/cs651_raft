package shardctrler

import (
	"log"
	"time"
)

//  +++ client side +++

func (ck *Clerk) Join(servers map[int][]string) {
	args := &JoinArgs{}
	// Your code here.

	// operation info
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++

	// request argument parameters
	args.Servers = servers

	for {
		// try each known server.
		for i, srv := range ck.servers {
			DPrintf("CK[%v]: Client side Join, to server index at: %v, RequestIndex: %v, Servers: %v\n",
				ck.clerkRandomId, i, args.RequestIndex, args.Servers)
			var reply JoinReply
			ok := srv.Call("ShardCtrler.Join", args, &reply)
			if ok && reply.WrongLeader == false {
				if reply.Err != OK {
					log.Fatalln("In client request side, wrong leader is true but err is not wrong leader bug.")
				}
				return
			}
			DPrintf("CK[%v]: result error: %v, Client side Join, to server index at: %v, RequestIndex: %v, Servers: %v\n",
				ck.clerkRandomId, reply.Err, i, args.RequestIndex, args.Servers)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// +++ server side +++

func (sc *ShardCtrler) creatJoinOp(args *JoinArgs) Op {
	op := Op{}
	op.ServerId = sc.me

	op.Type = JOIN

	// parameter
	op.Servers = args.Servers

	// identifier
	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

func (sc *ShardCtrler) preJoinStart(args *JoinArgs, reply *JoinReply) (Op, OpKVReplyChannel, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, trying to join.\n",
		sc.me)

	// create the op for this get
	op := sc.creatJoinOp(args)
	startCh := sc.createStartCh(op)

	return op, startCh, false
}

func (sc *ShardCtrler) handleJoinResult(op Op, responseMsg ReplyMsg, reply *JoinReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, op: %v, responseMsg: %v\n",
		sc.me, op, responseMsg)

	sc.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleJoinResult(), response error is ErrFiller, which should not be")
	}

	reply.Err = responseMsg.Error
	if reply.Err != OK {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
	}
}

func (sc *ShardCtrler) joinTimeout(op Op, reply *JoinReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		sc.me, op.ClientId, op.OpIndex)

	sc.deleteStartCh(op)

	reply.Err, reply.WrongLeader = ErrTimeout, true
	return
}

func (sc *ShardCtrler) Join(args *JoinArgs, reply *JoinReply) {
	// Your code here.
	if _, isLeader := sc.rf.GetState(); !isLeader {
		reply.Err, reply.WrongLeader = ErrWrongLeader, true
		return
	}
	DPrintf("SC[%v]: Join, server listener side, RequestIndex: %v, Servers: %v, ClientId: %v\n",
		sc.me, args.RequestIndex, args.Servers, args.ClientId)

	op, startCh, _ := sc.preJoinStart(args, reply)

	// set error first
	reply.Err, reply.WrongLeader = ErrOther, true

	//sc.rf.Start(op)
	_, _, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.Err = ErrWrongLeader
		sc.deleteStartChLocked(op)
		return
	}
	DPrintf("SPP> is leader, send op.")

	select {
	case responseMsg := <-startCh.ReplyChannel:
		sc.handleJoinResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		sc.joinTimeout(op, reply)
		return
	}
}
