package shardctrler

//
// Shardctrler clerk.
//

import "6.824/labrpc"
import "crypto/rand"
import "math/big"

type Clerk struct {
	servers []*labrpc.ClientEnd

	// avoid duplicate
	clerkRandomId int64
	requestIndex  int

	// for faster leader hit rate
	lastLeaderServer int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// Your code here.

	ck.clerkRandomId = nrand()
	ck.requestIndex = 0

	ck.lastLeaderServer = 0

	DPrintf("CCC= client: %v, client create with id: %v, len(servers): %v\n",
		ck.clerkRandomId, ck.clerkRandomId, len(servers))

	return ck
}
