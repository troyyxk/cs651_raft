package shardctrler

import (
	"log"
	"time"
)

//  +++ client side +++

func (ck *Clerk) Move(shard int, gid int) {
	args := &MoveArgs{}
	// Your code here.

	// operation info
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++

	// request argument parameters
	args.Shard = shard
	args.GID = gid

	for {
		// try each known server.
		for i, srv := range ck.servers {
			DPrintf("CK[%v]: Client side Move, to server index at: %v, RequestIndex: %v, Shard: %v, GID: %v\n",
				ck.clerkRandomId, i, args.RequestIndex, args.Shard, args.GID)
			var reply MoveReply
			ok := srv.Call("ShardCtrler.Move", args, &reply)
			if ok && reply.WrongLeader == false {
				if reply.Err != OK {
					log.Fatalln("In client request side, wrong leader is true but err is not wrong leader bug.")
				}
				return
			}
			DPrintf("CK[%v]: result error: %v, Client side Move, to server index at: %v, RequestIndex: %v, Shard: %v, GID: %v\n",
				ck.clerkRandomId, reply.Err, i, args.RequestIndex, args.Shard, args.GID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// +++ server side +++

func (sc *ShardCtrler) creatMoveOp(args *MoveArgs) Op {
	op := Op{}
	op.ServerId = sc.me

	op.Type = MOVE

	// parameter
	op.Shard = args.Shard
	op.GID = args.GID

	// identifier
	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

func (sc *ShardCtrler) preMoveStart(args *MoveArgs, reply *MoveReply) (Op, OpKVReplyChannel, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, trying to move.\n",
		sc.me)

	// create the op for this get
	op := sc.creatMoveOp(args)
	startCh := sc.createStartCh(op)

	return op, startCh, false
}

func (sc *ShardCtrler) handleMoveResult(op Op, responseMsg ReplyMsg, reply *MoveReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, op: %v, responseMsg: %v\n",
		sc.me, op, responseMsg)

	sc.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleMoveResult(), response error is ErrFiller, which should not be")
	}

	reply.Err = responseMsg.Error
	if reply.Err != OK {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
	}
}

func (sc *ShardCtrler) moveTimeout(op Op, reply *MoveReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		sc.me, op.ClientId, op.OpIndex)

	sc.deleteStartCh(op)

	reply.Err, reply.WrongLeader = ErrTimeout, true
	return
}

func (sc *ShardCtrler) Move(args *MoveArgs, reply *MoveReply) {
	// Your code here.
	if _, isLeader := sc.rf.GetState(); !isLeader {
		reply.Err, reply.WrongLeader = ErrWrongLeader, true
		return
	}

	DPrintf("SC[%v]: Join, server listener side, RequestIndex: %v, GID: %v, Shard: %v, ClientId: %v\n",
		sc.me, args.RequestIndex, args.GID, args.Shard, args.ClientId)

	op, startCh, _ := sc.preMoveStart(args, reply)

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
		sc.handleMoveResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		sc.moveTimeout(op, reply)
		return
	}
}
