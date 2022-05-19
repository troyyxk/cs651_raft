package kvraft

import (
	"6.824/labgob"
	"bytes"
	"log"
)

func (kv *KVServer) makeSnapshot() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(*(kv.state))
	e.Encode(kv.opIndex)
	e.Encode(kv.totalLogCount)
	data := w.Bytes()
	return data
}

func (kv *KVServer) readSnapshot(data []byte) (*State, int) {
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var newState State
	var snapShotOpIndex int
	var snapShotTotalLogCount int
	if d.Decode(&newState) != nil ||
		d.Decode(&snapShotOpIndex) != nil ||
		d.Decode(&snapShotTotalLogCount) != nil {
		log.Fatalln("read persist error")
	}
	return &newState, snapShotTotalLogCount
}

func (kv *KVServer) installSnapshot(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		DPrintf("In installSnapshot(), !!!trying to import empty!!!\n")
		return
	}
	kv.state, kv.totalLogCount = kv.readSnapshot(data)
	// each time reading a snapshot, set the count back to 0
	kv.instoreLogCount = 0
}
