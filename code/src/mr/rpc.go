package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.
type TaskType int

const (
	Map    TaskType = 0
	Reduce TaskType = 1
	Wait   TaskType = 2
	Done   TaskType = 3
)

type RequestType int

const (
	Get RequestType = 0
	Put RequestType = 1
)

type RequestTaskArgs struct {
	RequestType RequestType
	TaskType    TaskType
	TaskInd     int
}
type RequestTaskReply struct {
	Type    TaskType
	TaskInd int
	// For map only
	FileName string
	NReduce  int
	// For reduce only
	NMap int
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
