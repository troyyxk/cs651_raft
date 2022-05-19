package shardctrler

import (
	"log"
	"time"
)

//  +++ client side +++

func (ck *Clerk) Query(num int) Config {
	args := &QueryArgs{}
	// Your code here.

	// operation info
	args.ClientId = ck.clerkRandomId
	args.RequestIndex = ck.requestIndex
	ck.requestIndex++

	// request argument parameters
	args.Num = num

	for {
		// try each known server.
		for i, srv := range ck.servers {
			DPrintf("CK[%v]: Client side Query, to server index at: %v, RequestIndex: %v, Num: %v\n",
				ck.clerkRandomId, i, args.RequestIndex, args.Num)
			var reply QueryReply
			ok := srv.Call("ShardCtrler.Query", args, &reply)
			if ok && reply.WrongLeader == false {
				if reply.Err != OK {
					log.Fatalln("In client request side, wrong leader is true but err is not wrong leader bug.")
				}
				return reply.Config
			}
			DPrintf("CK[%v]: result error: %v, Client side Query, to server index at: %v, RequestIndex: %v, Num: %v\n",
				ck.clerkRandomId, reply.Err, i, args.RequestIndex, args.Num)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// +++ server side +++

func (sc *ShardCtrler) createQueryOp(args *QueryArgs) Op {
	op := Op{}
	op.ServerId = sc.me

	op.Type = QUERY

	// parameter
	op.Num = args.Num

	// identifier
	op.ClientId = args.ClientId
	op.RequestIndex = args.RequestIndex
	return op
}

func (sc *ShardCtrler) preQueryStart(args *QueryArgs, reply *QueryReply) (Op, OpKVReplyChannel, bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, num: %v, trying to query.\n",
		sc.me, args.Num)

	// create the op for this get
	op := sc.createQueryOp(args)
	startCh := sc.createStartCh(op)

	return op, startCh, false
}

func (sc *ShardCtrler) handleQueryResult(op Op, responseMsg ReplyMsg, reply *QueryReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, hear back from startCh, op: %v, responseMsg: %v\n",
		sc.me, op, responseMsg)

	sc.deleteStartCh(op)

	if responseMsg.Error == ErrFiller {
		log.Fatalln("In handleQueryResult(), response error is ErrFiller, which should not be")
	}

	reply.Err, reply.Config = responseMsg.Error, responseMsg.ReturnConfig
	if reply.Err != OK {
		reply.WrongLeader = true
	} else {
		reply.WrongLeader = false
	}
}

func (sc *ShardCtrler) queryTimeout(op Op, reply *QueryReply) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	DPrintf("SGG> kvserver: %v, does not hear back from start channel, timeout， op.ClientId： %v, op.OpIndex: %v\n",
		sc.me, op.ClientId, op.OpIndex)

	sc.deleteStartCh(op)

	reply.Err, reply.WrongLeader = ErrTimeout, true
	return
}

func (sc *ShardCtrler) Query(args *QueryArgs, reply *QueryReply) {
	if _, isLeader := sc.rf.GetState(); !isLeader {
		reply.Err, reply.WrongLeader = ErrWrongLeader, true
		return
	}

	DPrintf("SC[%v]: Join, server listener side, RequestIndex: %v, Num: %v, ClientId: %v\n",
		sc.me, args.RequestIndex, args.Num, args.ClientId)

	op, startCh, _ := sc.preQueryStart(args, reply)

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
		sc.handleQueryResult(op, responseMsg, reply)
		return
	case <-time.After(StartChannelWaitTime * time.Millisecond):
		sc.queryTimeout(op, reply)
		return
	}
}
