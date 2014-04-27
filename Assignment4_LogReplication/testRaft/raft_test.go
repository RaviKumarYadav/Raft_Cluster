package raft_test

import (
	"encoding/json"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

type PrintData struct {
}

func (pd *PrintData) Write(data []byte) (int, error) {

	//// Following has been commented intentionally so that we can control "the output of
	//// underlying program (invoked by exec.Command) being executed.
	//fmt.Println(string(data))

	var err error

	err = nil
	n := len(data)

	return n, err
}

type jsonobject struct {
	Server []ServerDetails
}

type ServerDetails struct {
	Server_Id int
	Port_Num  int
}

/***************************************
Struct to be used for Leader Election
****************************************/
type Connection struct {
	// Round of Check or To maintain the conversation in sync
	attempt int

	// Count no. of message sent in each iteration
	sendCount int

	// Count no. of message sent in each iteration
	recvCount int

	// No. of Leaders in a given attempt
	leaders int

	// ServerId of the Leader
	leaderId int

	// Own PortNo.
	selfPort int

	// Channel to be used to close desired/all server(s) eg:- Leader
	KillConn chan string

	// Channel to be used to close Receiver()
	stopGoRoutine chan string

	receivedMessage chan string
}

/************************************************************
	Creates Sockets for Sending and Receiving purpose
*************************************************************/
func createSendAndReceivingSocket(configFile string, selfPort int) ([]*zmq.Socket, *zmq.Socket, map[int]*zmq.Socket) {

	file, e := ioutil.ReadFile("./" + configFile)

	if e != nil {
		fmt.Printf("Raft Test File error: %v\n", e)
		os.Exit(1)
	}

	var jsontype jsonobject
	json.Unmarshal(file, &jsontype)

	elementCount := len(jsontype.Server)

	sendConnections := make([]*zmq.Socket, elementCount)
	serverId_Socket_Map := make(map[int]*zmq.Socket)

	tempId := 0

	for _, value := range jsontype.Server {

		sendConnections[tempId], _ = zmq.NewSocket(zmq.PUSH)
		sendConnections[tempId].Connect("tcp://localhost:" + strconv.Itoa(value.Port_Num+1))
		serverId_Socket_Map[value.Server_Id] = sendConnections[tempId]
		tempId++
	}

	var receiveConnection *zmq.Socket

	receiveConnection, _ = zmq.NewSocket(zmq.PULL)
	receiveConnection.Bind("tcp://*:" + strconv.Itoa(selfPort))

	return sendConnections, receiveConnection, serverId_Socket_Map

}

/************************************************************
	Creates and Run Each Server Instance
*************************************************************/
func runServer(goFile string, sId int, serverStateFile string, configFile string, minTimeout int, maxTimeout int, conn *Connection) {

	var pd PrintData

	env := os.Environ()

	cmd := exec.Command(goFile, strconv.Itoa(sId), serverStateFile, configFile, strconv.Itoa(minTimeout), strconv.Itoa(maxTimeout), strconv.Itoa(conn.selfPort))
	cmd.Env = env
	cmd.Stdout = &pd

	error := cmd.Start()

	if error != nil {
		fmt.Println("Error in Start --> ", error)
	}

	// Checking for the SERVER to be closed
	for {
		code := <-conn.KillConn
		kill_code, _ := strconv.Atoi(code)

		if kill_code == -1 || kill_code == sId {
			//fmt.Println("Killed Server ",sId)
			cmd.Process.Kill()
			return
		}
	}

}

/****************************************************************************************************************
	Scenario 	:- Testing KV Store Consistency after one server goes down and later restored ,
	Desired Outcome :- All Server have same copy of Data (KV Store)
******************************************************************************************************************/
func Test_Consistency_With_Killing_Then_ReStarting_One_Servers(t *testing.T) {

	fmt.Println("\n\n******************************************************************")
	fmt.Println("Testing Consistency_With_Killing_Then_ReStarting_One_Servers")
	fmt.Println("******************************************************************")

	var conn Connection

	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)
	conn.receivedMessage = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 300
	maxTimeout := 400

	//serverId_Socket_Map
	sendSockets, recSockets, serverId_Socket_Map := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)
	time.Sleep(1000 * time.Millisecond)
	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	fmt.Println("\nComputing Leader , Please bear with us...")
	// Time for Leader Election
	time.Sleep(1500 * time.Millisecond)

	// Send Message and Receive the Status for finding Leader
	for _, sock := range sendSockets {
		sock.Send("LeaderId", zmq.DONTWAIT)
	}

	// Wait for the Responses
	time.Sleep(1000 * time.Millisecond)

	var leaderId int

	for _, _ = range sendSockets {
		leaderId, _ = strconv.Atoi(strings.Split(<-conn.receivedMessage, ":")[1])
	}

	time.Sleep(500 * time.Millisecond)

	fmt.Println("\nCurrent Leader is Server", leaderId)
	leaderKilled := leaderId
	// kill Leader and the check after few seconds
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- strconv.Itoa(leaderKilled)
	}
	fmt.Println("\nLeader Killed i.e. Server ", leaderKilled)

	time.Sleep(1000 * time.Millisecond)

	// Send Message and Receive the Status for finding new Leader
	for tempKey, _ := range serverId_Socket_Map {

		if tempKey != leaderKilled {
			serverId_Socket_Map[tempKey].Send("LeaderId", zmq.DONTWAIT)
		}
	}

	// Wait for the Responses
	time.Sleep(500 * time.Millisecond)

	loopVar := 1
	for loopVar < noOfServers {
		leaderId, _ = strconv.Atoi(strings.Split(<-conn.receivedMessage, ":")[1])
		loopVar++
	}

	fmt.Println("\nNew Leader is Server", leaderId, "\n")
	time.Sleep(500 * time.Millisecond)

	// Pushing New Values after a new Leader was Elected
	// Send 10 "Put" request to New Leader
	i := 500
	j := 600
	counter := 1
	valueSentMap := make(map[int]int)

	// "Put"-request sent to Leader	, as "Put <i> <j>"
	fmt.Println("Sending Key-Values pair to remote Leader")
	for counter <= 10 {
		serverId_Socket_Map[leaderId].Send("Put "+strconv.Itoa(i)+" "+strconv.Itoa(j), zmq.DONTWAIT)
		valueSentMap[i] = j
		fmt.Println("key : ", i, " , value : ", j)
		i++
		j++
		counter++
	}

	time.Sleep(1000 * time.Millisecond)

	// Receive all Responses for earlier "Put" Request , not so fruitful (just clearing the channel)
	counter = 1
	for counter <= 10 {
		_ = strings.Split(<-conn.receivedMessage, ":")[1]
		counter++
	}

	time.Sleep(1000 * time.Millisecond)

	// Restarting the Old Stopped Server again
	fmt.Println("Restarting closed server ", leaderKilled)
	go runServer(goFile, leaderKilled, serverStateFile, configFile, minTimeout, maxTimeout, &conn)

	// Wait for few moments for this newly added server to get values
	time.Sleep(1000 * time.Millisecond)

	// Send "Get" Request to all Servers for checking the KVStore Status (for all earlier sent Put-keys)
	for tempKey, _ := range serverId_Socket_Map {
		// For each server fire "Get" request
		i = 500
		counter = 1
		for counter <= 10 {
			serverId_Socket_Map[tempKey].Send("Get "+strconv.Itoa(i), zmq.DONTWAIT)
			i++
			counter++
		}

		time.Sleep(1000 * time.Millisecond)

		i = 500
		counter = 1
		// Receive the Response (value for Get request) and check with what was sent
		for counter <= 10 {
			msg := <-conn.receivedMessage
			key, _ := strconv.Atoi(strings.Split(msg, ":")[1])
			value, _ := strconv.Atoi(strings.Split(msg, ":")[2])

			// If it does not matches for a single instance then just show the error
			if value != valueSentMap[key] {
				t.Errorf("\nValues Not Equal for key ", key, ", sent ", valueSentMap[key], " , received   ", value)
			}

			i++
			counter++
		}
	}

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"
	recSockets.Close()
	close(conn.stopGoRoutine)
	close(conn.receivedMessage)

	// Kill all the servers
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- "-1"
	}

	time.Sleep(100 * time.Millisecond)

}

/**************************************************************************************
	Scenario : Testing that One Leader is elected and Only Leader replies to Client , Follower just pass the Leader_Id
	Outcome  : Only ONE Leader exists , Follower also send Leader Id if requested by Client
**************************************************************************************/
func Test_Identify_Leader(t *testing.T) {

	fmt.Println("\n\n********************************************************************************")
	fmt.Println("Test : Identifying Leader , Follower also send Leader Id if requested by Client")
	fmt.Println("*********************************************************************************")

	var conn Connection

	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 300
	maxTimeout := 400

	sendSockets, recSockets, _ := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	conn.receivedMessage = make(chan string)
	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)
	time.Sleep(500 * time.Millisecond)
	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	fmt.Println("\nComputing Leader , Please bear with us...")

	time.Sleep(1000 * time.Millisecond)

	// Send Message and Receive the Status
	for _, sock := range sendSockets {
		sock.Send("LeaderId", zmq.DONTWAIT)
	}

	// time for gathering the Responses from all servers in Cluster
	time.Sleep(1000 * time.Millisecond)

	leaderId := strings.Split(<-conn.receivedMessage, ":")[1]
	leaderStatus := true

	if leaderId == strings.Split(<-conn.receivedMessage, ":")[1] && leaderId == strings.Split(<-conn.receivedMessage, ":")[1] {
		leaderStatus = true
	} else {
		leaderStatus = false
	}

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"
	recSockets.Close()
	close(conn.stopGoRoutine)
	close(conn.receivedMessage)

	// Kill all the servers
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- "-1"
	}

	time.Sleep(100 * time.Millisecond)

	if leaderStatus != true {
		t.Errorf("\nleaderStatus is \n", leaderStatus)
	}

}

/*************************************************************************************************
Scenario 	:- Testing Consistency of KV Store at all working machines when all are up.
Desired Outcome :- Only One Leader Should be There , all follower should have consistent copies.
**************************************************************************************************/
func Test_Consistency_With_All_Servers(t *testing.T) {

	fmt.Println("\n\n********************************************************************************")
	fmt.Println("Test : Checking Consistency with all Servers Running")
	fmt.Println("*********************************************************************************")

	var conn Connection

	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)
	conn.receivedMessage = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 300
	maxTimeout := 400

	sendSockets, recSockets, serverId_Socket_Map := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)
	time.Sleep(500 * time.Millisecond)
	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	time.Sleep(500 * time.Millisecond)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	fmt.Println("\nComputing Leader , Please bear with us...")

	// Time for Leader Election
	time.Sleep(1500 * time.Millisecond)

	// Send Message and Receive the Status
	for _, sock := range sendSockets {
		sock.Send("LeaderId", zmq.DONTWAIT)
	}

	// Wait for the Responses
	time.Sleep(500 * time.Millisecond)

	var leaderId int

	for _, _ = range sendSockets {
		leaderId, _ = strconv.Atoi(strings.Split(<-conn.receivedMessage, ":")[1])
	}

	fmt.Println("\nCurrent Leader is Server", leaderId)
	time.Sleep(500 * time.Millisecond)

	// Send 10 "Put" request to Server
	i := 10
	j := 20
	counter := 1
	valueSentMap := make(map[int]int)

	// "Put"-request sent to Leader	, as "Put <i> <j>"
	fmt.Println("Sending Key-Values pair to remote Leader")
	for counter <= 10 {
		serverId_Socket_Map[leaderId].Send("Put "+strconv.Itoa(i)+" "+strconv.Itoa(j), zmq.DONTWAIT)
		valueSentMap[i] = j
		fmt.Println("key : ", i, " , value : ", j)
		i++
		j++
		counter++
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	// Receive all Responses for earlier "Put" Request , not so fruitful (just clearing the channel)
	counter = 1
	for counter <= 10 {
		_ = strings.Split(<-conn.receivedMessage, ":")[1]
		counter++
	}

	time.Sleep(500 * time.Millisecond)

	// Send "Get" Request to all Servers for checking the KVStore Status (for all earlier sent Put-keys)
	for _, sock := range sendSockets {
		// For each server fire "Get" request
		i = 10
		counter = 1
		for counter <= 10 {
			sock.Send("Get "+strconv.Itoa(i), zmq.DONTWAIT)
			i++
			counter++
		}

		time.Sleep(700 * time.Millisecond)

		i = 10
		counter = 1
		// Receive the Response (value for Get request) and check with what was sent
		for counter <= 10 {
			msg := <-conn.receivedMessage
			key, _ := strconv.Atoi(strings.Split(msg, ":")[1])
			value, _ := strconv.Atoi(strings.Split(msg, ":")[2])

			// If it does not matches for a single instance then just show the error
			if value != valueSentMap[key] {
				t.Errorf("\nValues Not Equal for key ", key, ", sent ", valueSentMap[key], " , received   ", value)
			}

			i++
			counter++
		}
	}

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"
	recSockets.Close()
	close(conn.stopGoRoutine)
	close(conn.receivedMessage)

	// Kill all the servers
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- "-1"
	}

	time.Sleep(100 * time.Millisecond)

	if true != true {
		t.Errorf("Just Checking")
	}

}

/*************************************************************************************
	Scenario 	:- Testing Consistency of KV store when one out of three machine goes down ,
	Desired Outcome :- Only One Leader Should be There , On crash of one server other two must make a new leader (as Cluster size is three)
**************************************************************************************/
func Test_Consistency_With_One_Server_Killed(t *testing.T) {

	fmt.Println("\n\n*********************************************")
	fmt.Println("Testing Consistency_With_One_Server_Killed")
	fmt.Println("*********************************************")

	var conn Connection

	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)
	conn.receivedMessage = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 300
	maxTimeout := 400

	//serverId_Socket_Map
	sendSockets, recSockets, serverId_Socket_Map := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)
	time.Sleep(500 * time.Millisecond)
	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	fmt.Println("\nComputing Leader , Please bear with us...")
	// Time for Leader Election
	time.Sleep(1500 * time.Millisecond)

	// Send Message and Receive the Status for finding Leader
	for _, sock := range sendSockets {
		sock.Send("LeaderId", zmq.DONTWAIT)
	}

	// Wait for the Responses
	time.Sleep(1000 * time.Millisecond)

	var leaderId int

	for _, _ = range sendSockets {
		leaderId, _ = strconv.Atoi(strings.Split(<-conn.receivedMessage, ":")[1])
	}

	fmt.Println("\nCurrent Leader is Server", leaderId)
	leaderKilled := leaderId
	// kill Leader and the check after few seconds
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- strconv.Itoa(leaderKilled)
	}
	fmt.Println("\nLeader Killed i.e. Server ", leaderKilled, "\n\n")

	time.Sleep(1000 * time.Millisecond)

	// Send Message and Receive the Status for finding Leader
	for tempKey, _ := range serverId_Socket_Map {

		if tempKey != leaderKilled {
			serverId_Socket_Map[tempKey].Send("LeaderId", zmq.DONTWAIT)
		}
	}

	// Wait for the Responses
	time.Sleep(500 * time.Millisecond)

	loopVar := 1
	for loopVar < noOfServers {
		leaderId, _ = strconv.Atoi(strings.Split(<-conn.receivedMessage, ":")[1])
		loopVar++
	}

	fmt.Println("\nCurrent Leader is Server", leaderId)

	// Send 10 "Put" request to New Leader
	i := 100
	j := 200
	counter := 1
	valueSentMap := make(map[int]int)

	// "Put"-request sent to Leader	, as "Put <i> <j>"
	fmt.Println("Sending Key-Values pair to remote Leader")
	for counter <= 10 {
		serverId_Socket_Map[leaderId].Send("Put "+strconv.Itoa(i)+" "+strconv.Itoa(j), zmq.DONTWAIT)
		valueSentMap[i] = j
		fmt.Println("key : ", i, " , value : ", j)
		i++
		j++
		counter++
	}

	time.Sleep(1000 * time.Millisecond)

	fmt.Println("Received Message")
	// Receive all Responses for earlier "Put" Request , not so fruitful (just clearing the channel)
	counter = 1
	for counter <= 10 {
		_ = strings.Split(<-conn.receivedMessage, ":")[1]
		//fmt.Println(strings.Split(<-conn.receivedMessage,":")[1])
		counter++
	}

	time.Sleep(1000 * time.Millisecond)

	// Send "Get" Request to all Servers for checking the KVStore Status (for all earlier sent Put-keys)
	for tempKey, _ := range serverId_Socket_Map {
		// For each server fire "Get" request

		if tempKey != leaderKilled {

			i = 100
			counter = 1
			for counter <= 10 {
				serverId_Socket_Map[tempKey].Send("Get "+strconv.Itoa(i), zmq.DONTWAIT)
				i++
				counter++
			}

			time.Sleep(1000 * time.Millisecond)

			i = 100
			counter = 1
			// Receive the Response (value for Get request) and check with what was sent
			for counter <= 10 {
				msg := <-conn.receivedMessage
				key, _ := strconv.Atoi(strings.Split(msg, ":")[1])
				value, _ := strconv.Atoi(strings.Split(msg, ":")[2])

				// If it does not matches for a single instance then just show the error
				if value != valueSentMap[key] {
					t.Errorf("\nValues Not Equal for key ", key, ", sent ", valueSentMap[key], " , received   ", value)
				}

				i++
				counter++
			}
		}
	}

	//}

	fmt.Println("Before Closing 1")

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"
	recSockets.Close()
	close(conn.stopGoRoutine)
	close(conn.receivedMessage)

	// Kill all the servers
	for i := 0; i < noOfServers-1; i++ {
		conn.KillConn <- "-1"
	}

	time.Sleep(100 * time.Millisecond)

	if true != true {
		t.Errorf("Just Checking")
	}

}

/****************************************************************
	Receiving Message from all Servers
****************************************************************/
func receiveMessage(recSockets *zmq.Socket, conn *Connection) {
	fmt.Println("In Received")
	for {
		select {
		// Stopping Go Routine
		case _, _ = <-conn.stopGoRoutine:
			return

		default:
			recSockets.SetRcvtimeo(500 * time.Millisecond)
			msg, err := recSockets.Recv(0)

			if err == nil {

				//fmt.Println("Received :- ",msg)

				conn.receivedMessage <- msg
			}
		}
	}
}
