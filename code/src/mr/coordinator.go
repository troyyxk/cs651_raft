package mr

import (
	"fmt"
	"log"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

type TaskStatus int

const (
	InStore  TaskStatus = 0
	OnFly    TaskStatus = 1
	Finished TaskStatus = 2
)

type Coordinator struct {
	// Your definitions here.
	// lock
	mu   sync.Mutex
	cond *sync.Cond
	// files
	files []string
	// map task metadata
	nMap           int
	mapHandoutTime []time.Time
	mapStatus      []TaskStatus
	// reduce task metadata
	nReduce           int
	reduceHandoutTime []time.Time
	reduceStatus      []TaskStatus
	// done status
	isDone bool
}

// Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// check if all map tasks are done
func (c *Coordinator) allMapDone() bool {
	for _, status := range c.mapStatus {
		if status != Finished {
			return false
		}
	}
	return true
}

// check if all reduce tasks are done
func (c *Coordinator) allReduceDone() bool {
	for _, status := range c.reduceStatus {
		if status != Finished {
			return false
		}
	}
	return true
}

// check if all tasks are done
func (c *Coordinator) allDone() bool {
	return c.allMapDone() && c.allReduceDone()
}

// rpc handler
func (c *Coordinator) HandleRequestTask(args *RequestTaskArgs, reply *RequestTaskReply) error {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()
	if args.RequestType == Put {
		// put request
		if args.TaskType == Map {
			fmt.Printf("In coordinator.go, map task done, task index: %v\n", args.TaskInd)
			c.mapStatus[args.TaskInd] = Finished
		} else {
			fmt.Printf("In coordinator.goreduce task done, task index: %v\n", args.TaskInd)
			c.reduceStatus[args.TaskInd] = Finished
			if c.allDone() {
				fmt.Println("In coordinator.go, all task done!")
				c.isDone = true
			}
		}
		return nil
	} else {
		// get request
		fmt.Println("In coordinator.go, Receive HandleRequestTask")
		if c.allMapDone() && c.allReduceDone() {
			c.isDone = true
			reply.Type = Done
			return nil
		}
		// populate reply for map
		reply.Type = Map
		reply.NReduce = c.nReduce
		if !c.allMapDone() {
			jobGranted := false
			for i, status := range c.mapStatus {
				if status == InStore {
					// the task value of the task
					reply.TaskInd = i
					reply.FileName = c.files[i]
					// update information of the task
					c.mapHandoutTime[i] = time.Now()
					c.mapStatus[i] = OnFly
					jobGranted = true
					fmt.Printf("In coordinator.go, new map task created, task index: %v, filename %v\n", reply.TaskInd, reply.FileName)
					break
				} else if status == OnFly {
					// if task not finished in 10 seconds, assign it to someone else
					if c.mapHandoutTime[i].Before(time.Now().Add((-10) * time.Second)) {
						// the task value of the task
						reply.TaskInd = i
						reply.FileName = c.files[i]
						// update information of the task
						c.mapHandoutTime[i] = time.Now()
						c.mapStatus[i] = OnFly
						jobGranted = true
						fmt.Printf("In coordinator.go, retry map task created, task index: %v, filename %v\n", reply.TaskInd, reply.FileName)
						break
					}
				}
			}
			if !jobGranted {
				reply.Type = Wait
			}
			return nil
		}
		// populate reply for reduce
		reply.Type = Reduce
		reply.NMap = c.nMap
		if !c.allReduceDone() {
			jobGranted := false
			for i, status := range c.reduceStatus {
				if status == InStore {
					// the task value of the task
					reply.TaskInd = i
					// update information of the task
					c.reduceHandoutTime[i] = time.Now()
					c.reduceStatus[i] = OnFly
					jobGranted = true
					fmt.Printf("In coordinator.go, new reduce task created, task index: %v\n", reply.TaskInd)
					break
				} else if status == OnFly {
					// if task not finished in 10 seconds, assign it to someone else
					if c.reduceHandoutTime[i].Before(time.Now().Add((-10) * time.Second)) {
						// the task value of the task
						reply.TaskInd = i
						// update information of the task
						c.reduceHandoutTime[i] = time.Now()
						c.reduceStatus[i] = OnFly
						jobGranted = true
						fmt.Printf("In coordinator.go, new reduce task created, task index: %v\n", reply.TaskInd)
						break
					}
				}
			}
			if !jobGranted {
				reply.Type = Wait
			}
			return nil
		}
		// if it reaches here, it means no task handed out, all task done, the
		// program terminate
		c.isDone = true
		reply.Type = Done
		return nil
	}
}

//
// start a thread that listens for RPCs from worker.go
//
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
//
func (c *Coordinator) Done() bool {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()

	return c.isDone
}

//
// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{}

	// Your code here.
	c.cond = sync.NewCond(&c.mu)
	c.files = files
	c.nMap = len(c.files)
	c.mapStatus = make([]TaskStatus, c.nMap)
	for i := 1; i < c.nMap; i++ {
		c.mapStatus[i] = InStore
	}
	c.mapHandoutTime = make([]time.Time, c.nMap)
	c.nReduce = nReduce
	c.reduceStatus = make([]TaskStatus, c.nReduce)
	for i := 1; i < c.nReduce; i++ {
		c.reduceStatus[i] = InStore
	}
	c.reduceHandoutTime = make([]time.Time, c.nReduce)
	c.isDone = false

	c.server()
	return &c
}
