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

/*
// Send Functionality-----------------------------
sender_array[id] , _ = zmq.NewSocket(zmq.PUSH)
sender_array[id].Connect("tcp://localhost:"+ fmt.Sprintf("%d",value.Port_Num))

mutex.Lock()
//sender_array[id].Send("Hi " + fmt.Sprintf("%d",jsontype.Object.Server[id].Server_Id),0)
sender_array[id].Send("Hi from Server " + fmt.Sprintf("%d",self_server_Id),0)		
id = id + 1
fmt.Println("Sent Hi to " + fmt.Sprintf("%d",value.Server_Id))
mutex.Unlock()
// ----------------------------------------------


sId,_ := strconv.Atoi(os.Args[1])
self_server_Id := jsontype.Object.Server[index].Server_Id
fmt.Println(self_server_Id)

for _ , value := range jsontype.Object.Server {
	if self_server_Id != value.Server_Id {
		fmt.Println("\n "+fmt.Sprintf("%d",self_server_Id) + "  " + fmt.Sprintf("%d",value.Server_Id) + "...\n")
		sender_array[id] , _ = zmq.NewSocket(zmq.PUSH)
		sender_array[id].Connect("tcp://localhost:"+ fmt.Sprintf("%d",value.Port_Num))

		mutex.Lock()
		//sender_array[id].Send("Hi " + fmt.Sprintf("%d",jsontype.Object.Server[id].Server_Id),0)
		sender_array[id].Send("Hi from Server " + fmt.Sprintf("%d",self_server_Id),0)		
		id = id + 1
		fmt.Println("Sent Hi to " + fmt.Sprintf("%d",value.Server_Id))
		mutex.Unlock()
		}
}


}

*/

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

	//var sender_array [2]*zmq.Socket
	//sender_array := make([]*zmq.Socket,2)
	//id := 0

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

	/*
		sIdToIndex := make(map[int]int)
		for index , sId := range server.peers {
			sIdToIndex[sId] = index
		}
	*/

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
		//fmt.Println("\nreceived")
		// If Message/Envelope was intended to it only or Message was broadcasted
		//if pId == server.serverID ||  pId == -1 {
		//if pId == server.Pid() ||  pId == -1 {
		server.Inbox() <- &Envelope{Pid: pId, MsgId: int64(msgId), Msg: splittedStr[2]}
		//}

		/*	_ , ok := sIdToIndex[pId]
			fmt.Println("Received " + strconv.Itoa(pId) )
			if ok == true {
				server.Inbox() <- &Envelope{Pid:pId , MsgId:int64(msgId) , Msg:splittedStr[2]}
			}
		*/
	}

	//time.Sleep(time.Second*5)
}

//fmt.Printf("%s\n", string(file))

//var jsontype jsonobject
//json.Unmarshal(file, &jsontype)
//fmt.Printf("Results: %v\n", jsontype)

//index,_ := strconv.Atoi(os.Args[1])
//self_server_Id := jsontype.Object.Server[index].Server_Id
//fmt.Println(self_server_Id)

//CreateServer(self_server_Id,jsontype,index)
// Infinite Loop
//for {
//fmt.Println("/n Server Id %d",self_server_Id)
//time.Sleep(time.Second*10)
//}

//}

/*

func CreateServer(self_server_Id int , jsontype jsonobject , index int){
// Receiving Port ()
// zmq.PULL is implemented
go func(){
fmt.Printf("In PULL \n")
receiver, _ := zmq.NewSocket(zmq.PULL)
//defer receiver.Close()
receiver.Bind("tcp://*:"+ fmt.Sprintf("%d",jsontype.Object.Server[index].Port_Num))
fmt.Println("Self Port--> "+ fmt.Sprintf("%d",jsontype.Object.Server[index].Port_Num))
for {
msg , _  := receiver.Recv(0)
fmt.Printf("\nReceived Message "+msg)
//fmt.Printf("\nReceived Message ")
}
//time.Sleep(time.Second*5)
}()


// Sending Port ()
// zmq.PUSH is implemented
go func(){
fmt.Printf("In PUSH \n")

//var sender_array [2]*zmq.Socket
sender_array := make([]*zmq.Socket,2)
id := 0

	for _ , value := range jsontype.Object.Server {
		//fmt.Println("\n "+fmt.Sprintf("%d",self_server_Id) + "  " + fmt.Sprintf("%d",value.Server_Id))
		if self_server_Id != value.Server_Id {
		fmt.Println("\n "+fmt.Sprintf("%d",self_server_Id) + "  " + fmt.Sprintf("%d",value.Server_Id) + "...\n")
		sender_array[id] , _ = zmq.NewSocket(zmq.PUSH)
		sender_array[id].Connect("tcp://localhost:"+ fmt.Sprintf("%d",value.Port_Num))

		mutex.Lock()
		//sender_array[id].Send("Hi " + fmt.Sprintf("%d",jsontype.Object.Server[id].Server_Id),0)
		sender_array[id].Send("Hi from Server " + fmt.Sprintf("%d",self_server_Id),0)		
		id = id + 1
		fmt.Println("Sent Hi to " + fmt.Sprintf("%d",value.Server_Id))
		mutex.Unlock()
		}
		}

time.Sleep(time.Second*9)
}()
}

*/

/*
sender, _ := zmq.NewSocket(zmq.PUSH)
defer sender.Close()
sender.Connect("tcp://localhost:1000")

receiver, _ := zmq.NewSocket(zmq.PULL)
defer receiver.Close()
receiver.Bind("tcp://*:2000")


//fmt.Print("\n Server A , Press Enter when the workers are ready: ")

//Only for User Interrupt
var line string
fmt.Scanln(&line)


count := 0
i:="fdsfsfs"
go func () { msg , _ := receiver.Recv(0)
fmt.Println("Received from Server A %s\n",msg)
}()
sender.Send(i, 0)
fmt.Println(i)


fmt.Printf("Total Sent - %d \n", count)
time.Sleep(time.Second*10)
*/

/*
j := 0

	for i := 1 ; i < 10 ; i ++ {
		//if self_server_Id != value.Server_Id {
		fmt.Printf("\nSending Message 'Hi' ")
		sender_array[j].Send("Hi " + string(i),zmq.DONTWAIT)

		if j == (server_count-2) {
			j = 0
		} else {
			j = j+1
		}
		//
		}
	}
}()
*/
