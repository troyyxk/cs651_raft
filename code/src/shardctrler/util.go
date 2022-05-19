package shardctrler

import (
	"log"
	"sort"

	"6.824/raft"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

func (sc *ShardCtrler) isDuplicateOp(op Op) bool {
	lastRequestIndex, ok := sc.state.ClientLastRequest[op.ClientId]
	if ok {
		if op.RequestIndex <= lastRequestIndex {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}

func (sc *ShardCtrler) getValue(op Op) Config {
	if op.Type != QUERY {
		log.Fatalln("In geValue(), op type not QUERY")
	}
	if op.Num < 0 || op.Num >= len(sc.state.Configs) {
		return sc.state.Configs[len(sc.state.Configs)-1]
	}
	return sc.state.Configs[op.Num]
}

func (sc *ShardCtrler) getLatestConfig() Config {
	return sc.state.Configs[len(sc.state.Configs)-1]
}

func (sc *ShardCtrler) makeShallowCopyConfig(oldConfig Config) Config {
	newConfig := Config{}

	newConfig.Num = oldConfig.Num
	for i := 0; i < NShards; i++ {
		newConfig.Shards[i] = oldConfig.Shards[i]
	}

	newConfig.Groups = map[int][]string{}
	for k, v := range oldConfig.Groups {
		curGroup := []string{}
		for _, groupName := range v {
			curGroup = append(curGroup, groupName)
		}
		newConfig.Groups[k] = curGroup
	}

	return newConfig
}

func (sc *ShardCtrler) makeNextConfig() Config {
	nextConfig := sc.makeShallowCopyConfig(sc.getLatestConfig())
	nextConfig.Num = nextConfig.Num + 1
	return nextConfig
}

func (sc *ShardCtrler) getStartCh(Operation Op) (OpKVReplyChannel, bool) {
	var fillerChannel OpKVReplyChannel
	clientMap, ok := sc.startChannels[Operation.ClientId]
	if !ok {
		return fillerChannel, false
	}
	startCh, ok := clientMap[Operation.RequestIndex]
	return startCh, ok
}

func (sc *ShardCtrler) signalStartCh(m raft.ApplyMsg, replyMsg ReplyMsg) {
	responseOp := m.Command.(Op)
	startCh, ok := sc.getStartCh(responseOp)
	if !ok {
		DPrintf("STT> kvserver: %v, channel for CommandIndex: %v does not exist\n",
			sc.me, m.CommandIndex)
		return
	}
	startCh.ReplyChannel <- replyMsg
	sc.deleteStartCh(responseOp)
}

func (sc *ShardCtrler) createStartChLocked(Operation Op) OpKVReplyChannel {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.createStartCh(Operation)
}

func (sc *ShardCtrler) createStartCh(Operation Op) OpKVReplyChannel {
	_, ok := sc.startChannels[Operation.ClientId]
	if !ok {
		sc.startChannels[Operation.ClientId] = make(map[int]OpKVReplyChannel)
	}

	opChannel := OpKVReplyChannel{}
	opChannel.Operation = Operation
	opChannel.ReplyChannel = make(chan ReplyMsg)

	sc.startChannels[Operation.ClientId][Operation.RequestIndex] = opChannel

	return opChannel
}

func (sc *ShardCtrler) opChannelMatch(m raft.ApplyMsg) bool {
	responseOp := m.Command.(Op)

	if responseOp.ServerId != sc.me {
		return false
	}

	startCh, ok := sc.getStartCh(responseOp)
	if !ok {
		return false
	}
	originalOp := startCh.Operation

	if originalOp.ServerId != responseOp.ServerId ||
		originalOp.OpIndex != responseOp.OpIndex ||
		originalOp.Type != responseOp.Type ||
		originalOp.Shard != responseOp.Shard ||
		originalOp.GID != responseOp.GID ||
		originalOp.Num != responseOp.Num ||
		originalOp.ClientId != responseOp.ClientId ||
		originalOp.RequestIndex != responseOp.RequestIndex {
		DPrintf("In opChannelMatch(), key, value or type of the original op and response op does not match.")
		return false
	}

	return true
}

func (sc *ShardCtrler) deleteStartChLocked(op Op) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.deleteStartCh(op)
}

func (sc *ShardCtrler) deleteStartCh(op Op) {
	clientMap, ok := sc.startChannels[op.ClientId]
	if !ok {
		return
	}
	DPrintf("StartCh= kvserver: %v, try to delete start ch, before deletion, sc.startChannels: %v.\n",
		sc.me, sc.startChannels)
	startCh, ok := clientMap[op.RequestIndex]
	if ok {
		close(startCh.ReplyChannel)
		delete(clientMap, op.RequestIndex)
	}
	DPrintf("StartCh= kvserver: %v, after deletion, sc.startChannels: %v.\n",
		sc.me, sc.startChannels)
}

func getSortedKeys(curMap map[int][]string) []int {
	keys := make([]int, 0, len(curMap))
	for k := range curMap {
		keys = append(keys, k)
	}
	sort.Ints(keys[:])
	return keys
}

func contains(intArray []int, target int) bool {
	for _, value := range intArray {
		if value == target {
			return true
		}
	}
	return false
}

func assignVoidShards(config *Config) {
	keys := getSortedKeys(config.Groups)
	if len(keys) == 0 {
		DPrintf("length of keys is 0\n")
		return
	}

	curInd := 0
	for k, v := range config.Shards {
		if v <= 0 {
			config.Shards[k] = keys[curInd]
			curInd = (curInd + 1) % len(keys)
		}
	}
}

func getMinMaxGroup(config *Config) (int, int, int, int) {
	counts := map[int]int{}
	for _, g := range config.Shards {
		counts[g] += 1
	}
	var minGroup, maxGroup int
	min := 257
	max := 0
	gids := getSortedKeys(config.Groups)
	for _, g := range gids {
		if counts[g] > max {
			maxGroup = g
			max = counts[g]
		}
		if counts[g] < min {
			minGroup = g
			min = counts[g]
		}
	}
	return min, minGroup, max, maxGroup
}

func redistributeShards(config *Config) {
	var min, minGroup, max, maxGroup int
	min, minGroup, max, maxGroup = getMinMaxGroup(config)
	for max-min > 1 {
		for k, g := range config.Shards {
			if g == maxGroup {
				config.Shards[k] = minGroup
				break
			}
		}
		min, minGroup, max, maxGroup = getMinMaxGroup(config)
	}
}
