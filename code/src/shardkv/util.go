package shardkv

import (
	"log"
	"unsafe"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

func stringClone(s string) string {
	b := make([]byte, len(s))
	copy(b, s)
	return *(*string)(unsafe.Pointer(&b))
}

func clientLastRequestClone(sourceClientLastRequest map[int64]int) map[int64]int {
	resultClientLastRequest := make(map[int64]int)
	for key, value := range sourceClientLastRequest {
		resultClientLastRequest[key] = value
	}
	return resultClientLastRequest
}

func createEmptyKeyValueStore() KeyValueStore {
	newKVStore := KeyValueStore{}
	newKVStore.KvStore = make(map[string]string)

	return newKVStore
}

func (kvStore *KeyValueStore) clone() KeyValueStore {
	newKVStore := make(map[string]string)
	for k, v := range kvStore.KvStore {
		newKVStore[stringClone(k)] = stringClone(v)
	}
	resultKeyValueStore := KeyValueStore{newKVStore}
	return resultKeyValueStore
}

func (kv *ShardKV) createStartChLocked(Operation Op) OpKVReplyChannel {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	return kv.createStartCh(Operation)
}

func (kv *ShardKV) createStartCh(Operation Op) OpKVReplyChannel {
	_, ok := kv.startChannels[Operation.ClientId]
	if !ok {
		kv.startChannels[Operation.ClientId] = make(map[int]OpKVReplyChannel)
	}

	opChannel := OpKVReplyChannel{}
	opChannel.Operation = Operation
	opChannel.ReplyChannel = make(chan KVReplyMsg)

	kv.startChannels[Operation.ClientId][Operation.RequestIndex] = opChannel

	return opChannel
}

func (kv *ShardKV) getStartCh(op Op) (OpKVReplyChannel, bool) {
	var fillerChannel OpKVReplyChannel
	clientMap, ok := kv.startChannels[op.ClientId]
	if !ok {
		return fillerChannel, false
	}
	startCh, ok := clientMap[op.RequestIndex]
	return startCh, ok
}

func (kv *ShardKV) deleteStartChLocked(op Op) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.deleteStartCh(op)
}

func (kv *ShardKV) deleteStartCh(op Op) {
	clientMap, ok := kv.startChannels[op.ClientId]
	if !ok {
		return
	}
	DPrintf("StartCh= kvserver: %v, try to delete start ch, before deletion, op.ClientId: %v, op.RequestIndex: %v, kv.startChannels: %v.\n",
		kv.me, op.ClientId, op.RequestIndex, kv.startChannels)
	startCh, ok := clientMap[op.RequestIndex]
	if ok {
		close(startCh.ReplyChannel)
		delete(clientMap, op.RequestIndex)
	}
	DPrintf("StartCh= ShardKV: %v, after deletion, kv.startChannels: %v.\n",
		kv.me, kv.startChannels)
}

func (kv *ShardKV) isDuplicateOp(op Op) bool {
	lastRequestIndex, ok := kv.state.ClientLastRequest[op.ClientId]
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

func (kv *ShardKV) getValue(op Op) (string, Err) {
	if op.Type != GET {
		log.Fatalln("In geValue(), op type not GET")
	}
	if val, ok := kv.getValueWithKey(op.Key); ok {
		return val, OK
	} else {
		return "", ErrNoKey
	}
}

func (kv *ShardKV) containShardLocked(key string) bool {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	return kv.containShard(key)
}

func (kv *ShardKV) containShard(key string) bool {
	shardInd := key2shard(key)
	if kv.state.LatestConfig.Num == 0 {
		log.Fatalln("Haven't receive config yet")
	}
	if kv.state.LatestConfig.Shards[shardInd] == kv.gid {
		return true
	}
	return false
}

func (kv *ShardKV) getValueWithKey(key string) (string, bool) {
	shardInd := key2shard(key)
	shardKVStore, ok := kv.state.ShardKVStore[shardInd]
	if !ok {
		return "", false
	}
	value, ok := shardKVStore.KvStore[key]
	return value, ok
}

func (kv *ShardKV) setKeyValue(key string, value string) {
	shardInd := key2shard(key)
	_, ok := kv.state.ShardKVStore[shardInd]
	if !ok {
		newKeyValueStore := KeyValueStore{}
		kvStore := make(map[string]string)
		newKeyValueStore.KvStore = kvStore
		kv.state.ShardKVStore[shardInd] = newKeyValueStore
	}
	kv.state.ShardKVStore[shardInd].KvStore[key] = value
}

func (kv *ShardKV) curShardUpToDate(sid int) bool {
	if kv.state.LatestConfig.Shards[sid] != kv.gid {
		log.Fatalf("server [%v] sid [%v]; In curShardUpToDate(), not for this group [%v], LatestConfig.Shards: %v.\n",
			kv.me, sid, kv.gid, kv.state.LatestConfig.Shards)
	}
	// because of snapshot
	//if kv.state.ShardTimestamp[sid] > kv.state.LatestConfig.Num {
	//	log.Fatalf("server [%v] sid [%v]; In curShardUpToDate(), shard timestamp surpass config timestamp: %v, LatestConfig.Shards: %v.\n",
	//		kv.me, sid, kv.state.LatestConfig.Num, kv.state.ShardTimestamp)
	//}
	return kv.state.ShardTimestamp[sid] >= kv.state.LatestConfig.Num
}

// return not up to date shard, return true if all shard up to date
func (kv *ShardKV) allShardsUpToDate() ([]int, bool) {
	allUpToDate := true
	laggingShards := make([]int, 0)
	for sid, curGid := range kv.state.LatestConfig.Shards {
		if curGid == kv.gid {
			if !kv.curShardUpToDate(sid) {
				laggingShards = append(laggingShards, sid)
				allUpToDate = false
			}
		}
	}
	return laggingShards, allUpToDate
}

func (kv *ShardKV) allShardsUpToDateLocked() ([]int, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	return kv.allShardsUpToDate()
}
