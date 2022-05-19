package shardctrler

import (
	"sync"

	"6.824/labgob"
	"6.824/labrpc"
	"6.824/raft"
)

type ShardCtrler struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	// Your data here.

	state         *State
	startChannels map[int64]map[int]OpKVReplyChannel
	//configs       []Config // indexed by config num
}

//
// the tester calls Kill() when a ShardCtrler instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (sc *ShardCtrler) Kill() {
	sc.rf.Kill()
	// Your code here, if desired.
}

// needed by shardkv tester
func (sc *ShardCtrler) Raft() *raft.Raft {
	return sc.rf
}

func (sc *ShardCtrler) makeState() *State {
	// make config
	configs := make([]Config, 1)
	configs[0].Groups = map[int][]string{}
	configs[0].Num = 0
	//for i := 0; i < NShards; i++ {
	//	configs[0].Shards[i] = -1
	//}

	// make client last request
	clientLastRequest := make(map[int64]int)

	state := new(State)
	state.Configs = configs
	state.ClientLastRequest = clientLastRequest
	return state
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant shardctrler service.
// me is the index of the current server in servers[].
//
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister) *ShardCtrler {
	sc := new(ShardCtrler)
	sc.me = me

	//sc.configs = make([]Config, 1)
	//sc.configs[0].Groups = map[int][]string{}

	labgob.Register(Op{})
	sc.applyCh = make(chan raft.ApplyMsg)
	sc.rf = raft.Make(servers, me, persister, sc.applyCh)

	// Your code here.
	sc.state = sc.makeState()
	sc.startChannels = make(map[int64]map[int]OpKVReplyChannel)

	// read from snapshot
	sc.installSnapshot(persister.ReadSnapshot())

	go sc.applyTicker()

	DPrintf("StartServer, shard controller: %v, shard controller server create\n",
		sc.me)

	return sc
}
