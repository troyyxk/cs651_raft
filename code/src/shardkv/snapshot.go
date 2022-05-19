package shardkv

import (
	"6.824/labgob"
	"bytes"
	"log"
)

func (kv *ShardKV) makeSnapshot() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(*(kv.state))
	data := w.Bytes()
	return data
}

func (kv *ShardKV) readSnapshot(data []byte) *State {
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var newState State
	if d.Decode(&newState) != nil {
		log.Fatalln("read persist error")
	}
	return &newState
}

func (kv *ShardKV) installSnapshot(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		DPrintf("In installSnapshot(), !!!trying to import empty!!!\n")
		return
	}
	kv.state = kv.readSnapshot(data)
}
