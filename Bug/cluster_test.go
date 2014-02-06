package cluster_test

import (
	"fmt"
	cluster "github.com/RaviKumarYadav/Raft/cluster"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

var mutex1 = &sync.Mutex{}
var mutex2 = &sync.Mutex{}

func TestUnicastMsgCount(t *testing.T) {

	// Create 4 servers
	serverArray := make([]cluster.Server, 4)

	send_count := 0
	received_count := 0

	// Start the receiver first , so that we catch all the packets
	for tempServerId := 0; tempServerId <= 3; tempServerId++ {
		serverArray[tempServerId] = cluster.NewServer(tempServerId+1, "config.json")

		go cluster.SendMessage(serverArray[tempServerId])
		go cluster.ReceiveMessage(serverArray[tempServerId])



		// *********** Error Prone Code ********************
		
		// 1. Corrected code , which calls a function created below
		// Uncomment following line to work smoothly without exception (and comment option 2)
		// go receive_always(serverArray[tempServerId], &received_count)
		
		// 2. Errorneous code , which create a go-routine here itself and somehow throws exception
		// Comment following code to work smoothly and Uncomment option 1
		go func() {
			fmt.Println("Value of i :- " , tempServerId)
			_ = <-serverArray[tempServerId].Inbox()
			mutex1.Lock()
			received_count += 1
			mutex1.Unlock()
		}()
		
		// ********** Ends *********************************


	}

	for tempServerId := 0; tempServerId <= 3; tempServerId++ {

		count := rand.Intn(100)

		serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
		send_count += 1

		serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
		send_count += 1
		serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
		send_count += 1
		serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
		send_count += 1

		// Take few random values and send those many Envelopes
		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		fmt.Println("Sent all messages for Server ", serverArray[tempServerId].Pid())

	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}

}

func TestBroadcastMsgCount(t *testing.T) {

	// Create 4 servers
	serverArray := make([]cluster.Server, 4)

	send_count := 0
	received_count := 0

	//	all_send_sockets := make([][]sendSocket,4,3)

	// Start the receiver first , so that we catch all the packets
	for tempServerId := 0; tempServerId <= 3; tempServerId++ {
		serverArray[tempServerId] = cluster.NewServer(tempServerId+1, "config.json")

		// fmt.Println("Press Any Enter to Start ...")
		// var line string
		// fmt.Scanln(&line)

		go cluster.SendMessage(serverArray[tempServerId])
		go cluster.ReceiveMessage(serverArray[tempServerId])
		
		go receive_always(serverArray[tempServerId], &received_count)
	}
	
	

	for tempServerId := 0; tempServerId <= 3; tempServerId++ {

		// Put cluster.Envelope after implementing Pacakges
		for i := 1; i <= 50; i++ {
			serverArray[tempServerId].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		fmt.Println("Sent all messages for Server ", serverArray[tempServerId].Pid())
	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}

}

func receive_always(s cluster.Server, received_count *int) {
	for {
		_ = <-s.Inbox()
		mutex1.Lock()
		*received_count += 1
		mutex1.Unlock()
	}

}
