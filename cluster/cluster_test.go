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

var serverArray = make([]cluster.Server, 4)

var send_count = 0
var received_count = 0

// Testing only for UNICAST messaging
func TestUnicastMsgCount(t *testing.T) {

	send_count = 0
	received_count = 0

	fmt.Printf("\n\nTesting UNICAST ...\n")

	// Create 4 servers
	// Start the receiver first , so that we catch all the packets
	for id := 0; id <= 3; id++ {
		serverArray[id] = cluster.NewServer(id+1, "config.json")

		go cluster.SendMessage(serverArray[id])
		go cluster.ReceiveMessage(serverArray[id])

		go receive_always(serverArray[id], &received_count)
	}

	for id := 0; id <= 3; id++ {

		count := rand.Intn(100)

		serverArray[id].Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
		send_count += 1

		serverArray[id].Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
		send_count += 1
		serverArray[id].Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
		send_count += 1
		serverArray[id].Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
		send_count += 1

		// Take few random values and send those many Envelopes
		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
			send_count += 1
		}

		//fmt.Println("Sent all messages for Server ", serverArray[id].Pid())

	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	fmt.Printf("\nTesing UNICAST result")
	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}
	fmt.Printf("\n\n\n")

}

// Testingo only for BROADCAST messaging
func TestBroadcastMsgCount(t *testing.T) {

	// We have used previously defined and initialized serverArray for this test-routine.
	// We are working on already running "Sending" and "Receiving" "threads of each Server.

	send_count = 0

	mutex1.Lock()
	received_count = 0
	mutex1.Unlock()

	fmt.Printf("\n\nTesting BROADCAST ...\n")

	for id := 0; id <= 3; id++ {

		// Put cluster.Envelope after implementing Pacakges
		for i := 1; i <= 50; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		//fmt.Println("Sent all messages for Server ", serverArray[id].Pid())
	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	fmt.Printf("\nTesing BROADCAST result")
	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}
	fmt.Printf("\n\n\n")

}

// Testing only for UNICAST + MULTICAST messaging
func TestUnicastMulticastMsgCount(t *testing.T) {

	// We have used previously defined and initialized serverArray for this test-routine.
	// We are working on already running "Sending" and "Receiving" "threads of each Server.

	send_count = 0

	mutex1.Lock()
	received_count = 0
	mutex1.Unlock()

	fmt.Printf("\n\nTesting UNICAST + MULTICAST ...\n")

	for id := 0; id <= 3; id++ {

		// Put cluster.Envelope after implementing Pacakges
		for i := 1; i <= 50; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		//fmt.Println("Sent all messages for Server ", serverArray[id].Pid())

		count := rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 1, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 2, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 3, Msg: "hello there"}
			send_count += 1
		}

		count = rand.Intn(1000)
		for i := 1; i <= count; i++ {
			serverArray[id].Outbox() <- &cluster.Envelope{Pid: 4, Msg: "hello there"}
			send_count += 1
		}

	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	fmt.Printf("\nTesing UNICAST + MULTICAST result")
	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}
	fmt.Printf("\n\n\n")

	time.Sleep(1 * time.Second)
}

// Testing for CYCLIC UNICAST messaging (means Server sending messages in cyclic order)
func TestCyclicUnicastMsgCount(t *testing.T) {

	// We have used previously defined and initialized serverArray for this test-routine.
	// We are working on already running "Sending" and "Receiving" "threads of each Server.

	send_count = 0

	mutex1.Lock()
	received_count = 0
	mutex1.Unlock()

	fmt.Printf("\n\nTesting CYCLIC UNICAST ...\n")

	for id := 0; id <= 3; id++ {

		// Put cluster.Envelope into Outbox after implementing Packages
		for i := 1; i <= 500; i++ {
			serverArray[(id+1)%4].Outbox() <- &cluster.Envelope{Pid: cluster.BROADCAST, Msg: "hello there"}
			send_count += 3
		}

		//fmt.Println("Sent all messages for Server ", serverArray[id].Pid())
	}

	time.Sleep(2 * time.Second)

	fmt.Printf("\n--------Going to Count---------")
	fmt.Printf("\nTotal msg sent " + strconv.Itoa(send_count))
	fmt.Printf("\nReceived msg   " + strconv.Itoa(received_count))

	fmt.Printf("\nTesing CYCLIC UNICAST result")
	if send_count != received_count {
		t.Errorf("send_count - %v , received_count - %v", send_count, received_count)
	}
	fmt.Printf("\n\n\n")

}

func receive_always(s cluster.Server, received_count *int) {
	for {
		_ = <-s.Inbox()
		mutex1.Lock()
		*received_count += 1
		mutex1.Unlock()
	}

}
