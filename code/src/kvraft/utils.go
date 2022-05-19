package kvraft

import (
	"log"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

func (kv *KVServer) createStartChLocked(Operation Op) OpKVReplyChannel {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	return kv.createStartCh(Operation)
}

func (kv *KVServer) createStartCh(Operation Op) OpKVReplyChannel {
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

func (kv *KVServer) getStartCh(op Op) (OpKVReplyChannel, bool) {
	var fillerChannel OpKVReplyChannel
	clientMap, ok := kv.startChannels[op.ClientId]
	if !ok {
		return fillerChannel, false
	}
	startCh, ok := clientMap[op.RequestIndex]
	return startCh, ok
}

func (kv *KVServer) deleteStartChLocked(op Op) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.deleteStartCh(op)
}

func (kv *KVServer) deleteStartCh(op Op) {
	clientMap, ok := kv.startChannels[op.ClientId]
	if !ok {
		return
	}
	DPrintf("StartCh= kvserver: %v, try to delete start ch, before deletion, kv.startChannels: %v.\n",
		kv.me, kv.startChannels)
	startCh, ok := clientMap[op.RequestIndex]
	if ok {
		close(startCh.ReplyChannel)
		delete(clientMap, op.RequestIndex)
	}
	DPrintf("StartCh= kvserver: %v, after deletion, kv.startChannels: %v.\n",
		kv.me, kv.startChannels)
}

func (kv *KVServer) isDuplicateOp(op Op) bool {
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
