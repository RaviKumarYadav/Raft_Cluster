package main

import (
	"fmt"
	cluster "github.com/RaviKumarYadav/Raft"
	"os"
	"strconv"
	"time"
)

func main() {
	//file, e := ioutil.ReadFile("./config.json")

	//if e != nil {
	//fmt.Printf("File error: %v\n", e)
	//os.Exit(1)
	//}

	tempServerId, _ := strconv.Atoi(os.Args[1])
	server := cluster.NewServer(tempServerId, "config.json")

	fmt.Println("Press Any Enter to Start ...")
	var line string
	fmt.Scanln(&line)

	go cluster.SendMessage(server)
	go cluster.ReceiveMessage(server)

	// Put cluster.Envelope after implementing Pacakges
	server.Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 0, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: 5, Msg: "hello there"}
	server.Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}

	for {
		select {
		case envelope := <-server.Inbox():
			fmt.Printf("Received msg from %d: '%s'\n", envelope.Pid, envelope.Msg)

		case <-time.After(time.Second * 10):
			//println("Waited and waited. Ab thak gaya\n")
		}
	}

	//  fmt.Println("\n Sleeping for 5 sec" )
	//  time.Sleep(time.Second*5)

}
