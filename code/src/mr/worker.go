package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.

	var callResult bool
	for {
		// request task
		GetTaskArgs := RequestTaskArgs{}
		GetTaskArgs.RequestType = Get
		GetTaskReply := RequestTaskReply{}
		callResult = call("Coordinator.HandleRequestTask", &GetTaskArgs, &GetTaskReply)
		if !callResult || GetTaskReply.Type == Done {
			return
		}
		if GetTaskReply.Type == Map {
			fmt.Printf("In worker.go, get map task %v, filename %v\n", GetTaskReply.TaskInd, GetTaskReply.FileName)
		} else {
			fmt.Printf("In worker.go, get reduce task %v\n", GetTaskReply.TaskInd)
		}
		// if server done, assume job done, terminate worker process
		// there might also be a job done signal

		// do the task
		if GetTaskReply.Type == Map {
			DoMap(mapf, GetTaskReply.TaskInd, GetTaskReply.FileName, GetTaskReply.NReduce)
		} else if GetTaskReply.Type == Reduce {
			DoReduce(reducef, GetTaskReply.TaskInd, GetTaskReply.NMap)
		} else if GetTaskReply.Type == Wait {
			fmt.Println("In worker.go, wait 5s as no free tasks but not all tests are done.")
			time.Sleep(5 * time.Second)
			continue
		}

		// finish task
		DoneTaskArgs := RequestTaskArgs{}
		DoneTaskArgs.RequestType = Put
		DoneTaskArgs.TaskType = GetTaskReply.Type
		DoneTaskArgs.TaskInd = GetTaskReply.TaskInd
		DoneTaskReply := RequestTaskArgs{}
		callResult = call("Coordinator.HandleRequestTask", &DoneTaskArgs, &DoneTaskReply)
		if DoneTaskArgs.TaskType == Map {
			fmt.Printf("In worker.go, done map task %v\n", DoneTaskArgs.TaskInd)
		} else {
			fmt.Printf("In worker.go, done reduce task %v\n", DoneTaskArgs.TaskInd)
		}
		// if server done, assume job done, terminate worker process
		if !callResult {
			return
		}
	}

}

func GetIntermediateFileName(mapInd int, reduceInd int) string {
	out := fmt.Sprintf("mr-%v-%v", mapInd, reduceInd)
	return out
}

func DoMap(mapf func(string, string) []KeyValue, taskInd int, fileName string, nReduce int) {
	// read from the specific file
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("In worker.go, cannot open %v", fileName)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("In worker.go, cannot read %v", fileName)
	}
	file.Close()

	// compute intermediate data
	kva := mapf(fileName, string(content))

	// create intermediate file
	encs := make([]*json.Encoder, nReduce)
	tmpFiles := make([]*os.File, nReduce)
	tmpFileNames := make([]string, nReduce)

	for i := 0; i < nReduce; i++ {
		tmpFileName := GetIntermediateFileName(taskInd, i)
		f, err := ioutil.TempFile(".", tmpFileName+"*")
		if err != nil {
			log.Fatalf("In worker.go, cannot carete file %v", tmpFileName)
		}

		encs[i] = json.NewEncoder(f)
		tmpFiles[i] = f
		tmpFileNames[i] = tmpFileName
	}

	// store intermediate data in to the intermediate file
	for _, kv := range kva {
		reduceInd := ihash(kv.Key) % nReduce
		err := encs[reduceInd].Encode(&kv)
		if err != nil {
			log.Fatalf("In worker.go, encode error %v", fileName)
		}
	}

	// close files
	for i := 0; i < nReduce; i++ {
		oldName := tmpFiles[i].Name()
		err = os.Rename(oldName, tmpFileNames[i])
		if err != nil {
			log.Fatalf("In worker.go, cannot rename %v to %v", oldName, tmpFileNames[i])
		}
		err := tmpFiles[i].Close()
		if err != nil {
			log.Fatalf("In worker.go, cannot close %v", tmpFileNames[i])
		}
	}
}
func GetOutputFileName(reduceInd int) string {
	out := fmt.Sprintf("mr-out-%v", reduceInd)
	return out
}

func DoReduce(reducef func(string, []string) string, taskInd int, nMap int) {
	// all data in the bracket
	kva := []KeyValue{}

	// read intermediate files and get data
	for i := 0; i < nMap; i++ {
		// open intermediate file
		tmpFileName := GetIntermediateFileName(i, taskInd)
		file, err := os.Open(tmpFileName)
		if err != nil {
			log.Fatalf("In worker.go, cannot open intermediate file %v", tmpFileName)
		}

		// get data
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			kva = append(kva, kv)
		}
	}

	// copied from mrsequential.go
	sort.Sort(ByKey(kva))

	outputName := GetOutputFileName(taskInd)
	outputFile, err := ioutil.TempFile(".", "")
	if err != nil {
		panic(err)
		log.Fatalln("In worker.go, cannot create temperate file")
	}

	i := 0
	for i < len(kva) {
		j := i + 1
		for j < len(kva) && kva[j].Key == kva[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, kva[k].Value)
		}
		output := reducef(kva[i].Key, values)

		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(outputFile, "%v %v\n", kva[i].Key, output)

		i = j
	}
	err = os.Rename(outputFile.Name(), outputName)
	if err != nil {
		log.Fatalf("In worker.go, cannot rename %v to %v", outputFile.Name(), outputName)
	}
	err = outputFile.Close()
	if err != nil {
		log.Fatalf("In worker.go, cannot close file %v\n", outputName)
	}
}

//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Coordinator.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
