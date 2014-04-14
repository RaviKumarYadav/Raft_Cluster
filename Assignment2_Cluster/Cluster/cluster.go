package cluster

import (
	"encoding/json"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
)

type jsonobject struct {
	Server []ServerDetails
}

type ServerDetails struct {
	Server_Id int
	Port_Num  int
}

var mutex = &sync.Mutex{}

const (
	BROADCAST int = -1
)

const (
	AppendEntriesRequest  int = 0
	AppendEntriesResponse int = 1
	RequestVoteRequest    int = 2
	RequestVoteResponse   int = 3
)

const (
	VotedYes bool = true
	VotedNo  bool = false
)

type Envelope struct {
	// On the sender side, Pid identifies the receiving peer. If instead, Pid is
	// set to cluster.BROADCAST, the message is sent to all peers. On the receiver side, the
	// Id is always set to the original sender. If the Id is not found, the message is silently dropped
	Pid int

	// An id that globally and uniquely identifies the message, meant for duplicate detection at
	// higher levels. It is opaque to this package.
	MsgId int64

	Msg interface{}

	MsgType int

	SenderId int

	Term int

	Voted bool
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

func (s Server) ServerAddress() string {
	return s.serverAddress
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

/***********************************************************************
	Creates a new instance of Server based on ServerId passed 
	and its corresponding information stored in Config.json file
************************************************************************/
func NewServer(myId int, configFile string) Server {

	file, e := ioutil.ReadFile("./" + configFile)

	if e != nil {
		fmt.Printf("Cluster File error: %v\n", e)
		os.Exit(1)
	}

	var jsontype jsonobject
	json.Unmarshal(file, &jsontype)

	elementCount := len(jsontype.Server)

	var serverInfo Server

	serverInfo.serverID = myId
	serverInfo.peerAddress = make([]string, elementCount-1)
	serverInfo.peers = make([]int, elementCount-1)
	serverInfo.inbox = make(chan *Envelope)
	serverInfo.outbox = make(chan *Envelope)

	tempId := 0

	for index, value := range jsontype.Server {
		if serverInfo.serverID != value.Server_Id {

			serverInfo.peers[tempId] = value.Server_Id
			serverInfo.peerAddress[tempId] = "tcp://localhost:" + strconv.Itoa(value.Port_Num)
			tempId += 1

		} else {

			serverInfo.serverIndex = index
			serverInfo.serverAddress = "tcp://*:" + strconv.Itoa(value.Port_Num)

		}
	}

	return serverInfo

}

/**********************************************************************************************************************
	Sends the message over the network to the appropriate Server in cluster (as mention in Pid field of Message) ,
	Here message is taken out of the Outbox() channel and then sent to the concerned target.
**********************************************************************************************************************/
func SendMessage(server *Server) {

	sendSocket := make([]*zmq.Socket, len(server.peerAddress))
	sIdToSocketIndexMap := make(map[int]int)

	for index, sendToAddress := range server.peerAddress {
		sendSocket[index], _ = zmq.NewSocket(zmq.PUSH)
		sendSocket[index].Connect(sendToAddress)
		sIdToSocketIndexMap[server.peers[index]] = index
	}

	for {
		envelope := <-server.Outbox()

		message, _ := json.Marshal(&envelope)

		if envelope.Pid == -1 {
			// Broadcast Message
			for index, _ := range server.peerAddress {
				mutex.Lock()
				sendSocket[index].Send(string(message), zmq.DONTWAIT)
				mutex.Unlock()
			}

		} else {
			// Sending only to valid members in cluster only (Other than itself)
			// Unicast Message
			mutex.Lock()
			sendSocket[sIdToSocketIndexMap[envelope.Pid]].Send(string(message), zmq.DONTWAIT)
			mutex.Unlock()

		}
	}
}

/**********************************************************************************************************************
	Receives message meant for itself from the network ,
	Here message is pushed to the Inbox() channel and processed later.
**********************************************************************************************************************/
func ReceiveMessage(server *Server) {

	receiver, _ := zmq.NewSocket(zmq.PULL)
	receiver.Bind(server.serverAddress)

	for {
		var msg Envelope

		pkt_received, _ := receiver.Recv(0)
		json.Unmarshal([]byte(pkt_received), &msg)
		server.Inbox() <- &msg
	}

}
