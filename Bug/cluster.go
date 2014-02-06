//
// Server 1
// Binds PUSH socket to tcp://localhost:5557
// Sends batch of tasks to workers via that socket
//
package cluster

import (
	"encoding/json"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type jsonobject struct {
	Object ObjectType
}

type ObjectType struct {
	Total_Server int
	Server       []ServerType
}

type ServerType struct {
	Server_Id int
	Port_Num  int
}

var mutex = &sync.Mutex{}

const (
	BROADCAST = -1
)

type Envelope struct {
	// On the sender side, Pid identifies the receiving peer. If instead, Pid is
	// set to cluster.BROADCAST, the message is sent to all peers. On the receiver side, the
	// Id is always set to the original sender. If the Id is not found, the message is silently dropped
	Pid int

	// An id that globally and uniquely identifies the message, meant for duplicate detection at
	// higher levels. It is opaque to this package.
	MsgId int64

	// the actual message.
	Msg interface{}
}

type IServer interface {
	// Id of this server
	Pid() int

	// array of other servers' ids in the same cluster
	Peers() []int

	// the channel to use to send messages to other peers
	// Note that there are no guarantees of message delivery, and messages
	// are silently dropped
	Outbox() chan *Envelope

	// the channel to receive messages from other peers.
	Inbox() chan *Envelope
}

func (s Server) Pid() int {
	return s.serverID
}

func (s Server) Peers() []int {
	return s.peers
}

func (s Server) Outbox() chan *Envelope {
	return s.outbox
}

func (s Server) Inbox() chan *Envelope {
	return s.inbox
}

type Server struct {
	serverID int

	serverIndex int

	serverAddress string

	peers []int

	peerAddress []string

	inbox chan *Envelope

	outbox chan *Envelope
}

func NewServer(myId int, configFile string) Server {

	file, e := ioutil.ReadFile("./" + configFile)

	if e != nil {
		fmt.Printf("File error: %v\n", e)
		os.Exit(1)
	}

	var jsontype jsonobject
	json.Unmarshal(file, &jsontype)
	//fmt.Printf("Results: %v\n", jsontype)

	var serverInfo Server

	serverInfo.serverID = myId
	serverInfo.peers = make([]int, jsontype.Object.Total_Server-1)
	serverInfo.peerAddress = make([]string, jsontype.Object.Total_Server-1)
	serverInfo.inbox = make(chan *Envelope)
	serverInfo.outbox = make(chan *Envelope)

	tempId := 0

	for index, value := range jsontype.Object.Server {
		if serverInfo.serverID != value.Server_Id {

			//fmt.Println("\n " + strconv.Itoa(tempId) )
			serverInfo.peers[tempId] = value.Server_Id
			serverInfo.peerAddress[tempId] = "tcp://localhost:" + strconv.Itoa(value.Port_Num)
			tempId += 1

		} else {

			serverInfo.serverIndex = index
			serverInfo.serverAddress = "tcp://*:" + strconv.Itoa(value.Port_Num)

		}
	}

	//	time.Sleep(time.Second*9)
	return serverInfo

}

func SendMessage(server Server) {

	fmt.Printf("\n Going To Send Messages for Server " + strconv.Itoa(server.serverID) + "\n")

	sendSocket := make([]*zmq.Socket, len(server.peerAddress))
	sIdToSocketIndexMap := make(map[int]int)

	for index, sendToAddress := range server.peerAddress {
		//fmt.Println("\n "+fmt.Sprintf("%d",self_server_Id) + "  " + fmt.Sprintf("%d",value.Server_Id))
		sendSocket[index], _ = zmq.NewSocket(zmq.PUSH)
		sendSocket[index].Connect(sendToAddress)
		sIdToSocketIndexMap[server.peers[index]] = index
	}

	for {
		envelope := <-server.Outbox()
		if envelope.Pid == -1 {
			// Broadcast Message
			for index, _ := range server.peerAddress {
				mutex.Lock()
				sendSocket[index].Send(strconv.Itoa(envelope.Pid)+"::"+strconv.FormatInt(envelope.MsgId, 10)+"::"+envelope.Msg.(string), zmq.DONTWAIT)
				mutex.Unlock()
			}

		} else {
			// Sending only to valid members in cluster only (Other than itself)
			//_, ok := sIdToSocketIndexMap[envelope.Pid]
			// Including itself in OR condition to simply calculations in Testing Phase
			//if ok == true || envelope.Pid == server.Pid(){

			//fmt.Println("\nSending Packet to " + strconv.Itoa(envelope.Pid))
			// Unicast Message
			mutex.Lock()
			sendSocket[sIdToSocketIndexMap[envelope.Pid]].Send(strconv.Itoa(envelope.Pid)+"::"+strconv.FormatInt(envelope.MsgId, 10)+"::"+envelope.Msg.(string), zmq.DONTWAIT)
			mutex.Unlock()
			//}
		}
	}

	time.Sleep(time.Second * 9)
}

func ReceiveMessage(server Server) {

	fmt.Printf("\n Going To Receive Messages for Server " + strconv.Itoa(server.serverID) + "\n")

	receiver, _ := zmq.NewSocket(zmq.PULL)
	//defer receiver.Close()
	receiver.Bind(server.serverAddress)

	for {
		//fmt.Println("\nbefore received")
		msg, _ := receiver.Recv(0)
		//fmt.Println("\nafter received")
		splittedStr := strings.Split(msg, "::")
		pId, _ := strconv.Atoi(splittedStr[0])
		msgId, _ := strconv.Atoi(splittedStr[1])
		server.Inbox() <- &Envelope{Pid: pId, MsgId: int64(msgId), Msg: splittedStr[2]}
	}

	//time.Sleep(time.Second*5)
}
