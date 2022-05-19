package shardctrler

import (
	"6.824/labgob"
	"bytes"
	"log"
)

func (sc *ShardCtrler) makeSnapshot() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(*(sc.state))
	data := w.Bytes()
	return data
}

func (sc *ShardCtrler) readSnapshot(data []byte) *State {
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var newState State
	if d.Decode(&newState) != nil {
		log.Fatalln("read persist error")
	}
	return &newState
}

func (sc *ShardCtrler) installSnapshot(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		DPrintf("In installSnapshot(), !!!trying to import empty!!!\n")
		return
	}
	//fmt.Printf("Before read snapshot state: %v\n", sc.state)
	sc.state = sc.readSnapshot(data)
	//fmt.Printf("Read snapshot state: %v\n", sc.state)
}
