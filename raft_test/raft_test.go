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
}

/************************************************************
	Creates Sockets for Sending and Receiving purpose
*************************************************************/
func createSendAndReceivingSocket(configFile string, selfPort int) ([]*zmq.Socket, *zmq.Socket) {

	file, e := ioutil.ReadFile("./" + configFile)

	if e != nil {
		fmt.Printf("Raft Test File error: %v\n", e)
		os.Exit(1)
	}

	var jsontype jsonobject
	json.Unmarshal(file, &jsontype)

	elementCount := len(jsontype.Server)

	sendConnections := make([]*zmq.Socket, elementCount)

	tempId := 0

	for _, value := range jsontype.Server {

		sendConnections[tempId], _ = zmq.NewSocket(zmq.PUSH)
		sendConnections[tempId].Connect("tcp://localhost:" + strconv.Itoa(value.Port_Num+1))
		tempId++
	}

	var receiveConnection *zmq.Socket

	receiveConnection, _ = zmq.NewSocket(zmq.PULL)
	receiveConnection.Bind("tcp://*:" + strconv.Itoa(selfPort))

	return sendConnections, receiveConnection

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
			cmd.Process.Kill()
			return
		}
	}

}

/*********************************************************************
	Scenario : Testing for Leader Election with "All Servers UP"
	Outcome  : Only ONE Leader
*********************************************************************/
func TestLeaderWith_0_KilledServers(t *testing.T) {

	max := 1

	fmt.Println("\n\n******************************************")
	fmt.Println("Testing with ", max-1, " Leader killed")
	fmt.Println("******************************************")

	var conn Connection

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1
	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 600
	maxTimeout := 800

	sendSockets, recSockets := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1

	fmt.Println("\nComputing Leader , Please bear with us...")

	time.Sleep(2000 * time.Millisecond)

	// Send Message and Receive the Status
	for _, sock := range sendSockets {
		sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
		conn.sendCount += 1
	}

	// time for gathering the Responses from all servers in Cluster
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\nCurrent Leader is Server", conn.leaderId)

	// Kill all the servers
	for i := 0; i < noOfServers-max+1; i++ {
		conn.KillConn <- "-1"
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}
	recSockets.Close()

	if conn.leaders != 1 {
		t.Errorf("\nTotal Leaders - %v \n", conn.leaders)
	}

}

/*************************************************************************************
Scenario 	:- Testing for Leader Election with "One Server Down" , 
Desired Outcome :- Only One Leader Should be There
**************************************************************************************/
func TestLeaderWith_1_KilledServers(t *testing.T) {

	max := 2

	fmt.Println("\n\n******************************************")
	fmt.Println("Testing with ", max-1, " Leader killed")
	fmt.Println("******************************************")

	var conn Connection

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1
	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 600
	maxTimeout := 800

	sendSockets, recSockets := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	for i := 0; i < max; i++ {
		conn.attempt = i + 1
		conn.sendCount = 0
		conn.recvCount = 0
		conn.leaders = 0
		conn.leaderId = -1

		fmt.Println("\nComputing Leader , Please bear with us...")

		// Time for Leader Election
		time.Sleep(2000 * time.Millisecond)

		// Send Message and Receive the Status
		for _, sock := range sendSockets {
			sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
			conn.sendCount += 1
		}

		// Wait for the Responses
		time.Sleep(500 * time.Millisecond)

		fmt.Println("\n\nCurrent Leader is Server", conn.leaderId)

		// Find who is leader and then kill it and the check after few seconds		
		if i != max-1 {
			for i := 0; i < noOfServers; i++ {
				conn.KillConn <- strconv.Itoa(conn.leaderId)
			}
		}
	}

	// Kill all the servers
	for i := 0; i < noOfServers-max+1; i++ {
		conn.KillConn <- "-1"
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}
	recSockets.Close()

	if conn.leaders != 1 {
		t.Errorf("\nTotal Leaders - %v \n", conn.leaders)
	}
}

/*************************************************************************************
	Scenario 	:- Testing for Leader Election with "Two Server Down" , 
	Desired Outcome :- Only One Leader Should be There
**************************************************************************************/
func TestLeaderWith_2_KilledServers(t *testing.T) {

	max := 3

	fmt.Println("\n\n******************************************")
	fmt.Println("Testing with ", max-1, " Leader killed")
	fmt.Println("******************************************")

	var conn Connection

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1
	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 600
	maxTimeout := 800

	sendSockets, recSockets := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	for i := 0; i < max; i++ {
		conn.attempt = i + 1
		conn.sendCount = 0
		conn.recvCount = 0
		conn.leaders = 0
		conn.leaderId = -1

		fmt.Println("\nComputing Leader , Please bear with us...")
		// Time for Leader Election
		time.Sleep(2000 * time.Millisecond)

		// Send Message and Receive the Status
		for _, sock := range sendSockets {
			sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
			conn.sendCount += 1
		}

		// Wait for the Responses
		time.Sleep(500 * time.Millisecond)

		fmt.Println("\nCurrent Leader is Server", conn.leaderId)

		// Find who is leader and then kill it and the check after few seconds
		if i != max-1 {
			fmt.Println("\nKilling Server", conn.leaderId)
			for i := 0; i < noOfServers; i++ {
				conn.KillConn <- strconv.Itoa(conn.leaderId)
			}
		}
	}

	// Kill all the servers
	for i := 0; i < noOfServers-max+1; i++ {
		conn.KillConn <- "-1"
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}
	recSockets.Close()

	if conn.leaders != 1 {
		t.Errorf("\nTotal Leaders - %v \n", conn.leaders)
	}
}

/*************************************************************************************
	Scenario 	:- Testing for Leader Election with "Three Server Down" , 
	Desired Outcome :- NO Leader Should be There
**************************************************************************************/
func TestLeaderWith_3_KilledServers(t *testing.T) {

	max := 4

	fmt.Println("\n\n******************************************")
	fmt.Println("Testing with ", max-1, " Leader killed")
	fmt.Println("******************************************")

	var conn Connection

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1
	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 600
	maxTimeout := 800

	sendSockets, recSockets := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	for i := 0; i < max; i++ {
		conn.attempt = i + 1
		conn.sendCount = 0
		conn.recvCount = 0
		conn.leaders = 0
		conn.leaderId = -1

		fmt.Println("\nComputing Leader , Please bear with us...")
		// Time for Leader Election
		time.Sleep(2000 * time.Millisecond)

		// Send Message and Receive the Status
		for _, sock := range sendSockets {
			sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
			conn.sendCount += 1
		}

		// Wait for the Responses
		time.Sleep(500 * time.Millisecond)

		fmt.Println("\nCurrent Leader is Server", conn.leaderId)

		// Find who is leader and then kill it and the check after few seconds
		if i != max-1 {
			fmt.Println("\nKilling Server", conn.leaderId)
			for i := 0; i < noOfServers; i++ {
				if conn.leaderId == -1 {
					conn.KillConn <- "-2"
				} else {
					conn.KillConn <- strconv.Itoa(conn.leaderId)
				}
			}
		}
	}

	// Kill all the servers
	for i := 0; i < noOfServers-max+1; i++ {
		conn.KillConn <- "-1"
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}
	recSockets.Close()

	if conn.leaders != 0 {
		t.Errorf("\nTotal Leaders - %v \n", conn.leaders)
	}
}

/****************************************************************************************************************
	Scenario 	:- Testing for Leader Election with "Three Server Down" and later restarting one server, 
	Desired Outcome :- Only ONE Leader Should be There
******************************************************************************************************************/
func TestLeaderWith_3_KilledServers_1_RecoveredServer(t *testing.T) {

	max := 4

	fmt.Println("\n\n\n*******************************************************")
	fmt.Println("Testing with ", max-1, " Leader killed , One Recovered Server")
	fmt.Println("*******************************************************")

	var conn Connection
	recoveredServerId := -1

	conn.attempt = 1
	conn.sendCount = 0
	conn.recvCount = 0
	conn.leaders = 0
	conn.leaderId = -1
	conn.selfPort = 5678
	conn.KillConn = make(chan string)
	conn.stopGoRoutine = make(chan string)

	goFile := "../raft/raft"
	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeout := 600
	maxTimeout := 800

	sendSockets, recSockets := createSendAndReceivingSocket(configFile, conn.selfPort)
	noOfServers := len(sendSockets)

	// Receive Message(Leaders Status) from All Servers
	go receiveMessage(recSockets, &conn)

	// Launch all Servers
	for i := 0; i < noOfServers; i++ {
		go runServer(goFile, (i + 1), serverStateFile, configFile, minTimeout, maxTimeout, &conn)
	}

	for i := 0; i < max; i++ {
		conn.attempt = i + 1
		conn.sendCount = 0
		conn.recvCount = 0
		conn.leaders = 0
		conn.leaderId = -1

		fmt.Println("\nComputing Leader , Please bear with us...")
		// Time for Leader Election
		time.Sleep(2000 * time.Millisecond)

		// Send Message and Receive the Status
		for _, sock := range sendSockets {
			sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
			conn.sendCount += 1
		}

		// Wait for the Responses
		time.Sleep(500 * time.Millisecond)

		fmt.Println("\nCurrent Leader is Server", conn.leaderId)

		// Find who is leader and then kill it and the check after few seconds
		if i != max-1 {
			fmt.Println("\nKilling Server", conn.leaderId)
			for i := 0; i < noOfServers; i++ {
				conn.KillConn <- strconv.Itoa(conn.leaderId)
				recoveredServerId = conn.leaderId
			}
		}
	}

	fmt.Println("\nRestarting one Server.")
	// Restarting One Killed Server
	go runServer(goFile, recoveredServerId, serverStateFile, configFile, minTimeout, maxTimeout, &conn)

	fmt.Println("\nComputing Leader , Please bear with us...")
	// Time for Leader Election
	time.Sleep(2000 * time.Millisecond)

	conn.attempt = conn.attempt + 1

	// Send Message and Receive the Status
	for _, sock := range sendSockets {
		sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
		conn.sendCount += 1
	}

	// Wait for the Responses
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\nCurrent Leader is Server", conn.leaderId)

	// Kill all the servers , +1 for making it to "max-1" and +1 for extra server started earlier
	for i := 0; i < noOfServers-max+2; i++ {
		conn.KillConn <- "-1"
	}

	// Stop the Receive() go-routine
	conn.stopGoRoutine <- "STOP"

	// Close all Send Sockets
	for _, sock := range sendSockets {
		sock.Close()
	}
	recSockets.Close()

	if conn.leaders != 1 {
		t.Errorf("\nTotal Leaders - %v \n", conn.leaders)
	}

}

/****************************************************************
	Receiving Message from all Servers
****************************************************************/
func receiveMessage(recSockets *zmq.Socket, conn *Connection) {

	for {
		select {
		// Stopping Go Routine
		case _, _ = <-conn.stopGoRoutine:

			return

		default:
			recSockets.SetRcvtimeo(500 * time.Millisecond)
			msg, err := recSockets.Recv(0)

			if err == nil {
				msg_array := strings.Split(msg, ":")

				msg_seq := msg_array[0]
				isLeader := msg_array[1]
				leaderId, _ := strconv.Atoi(msg_array[2])

				count, _ := strconv.Atoi(isLeader)

				// Added the LeaderCount if the response belongs to corresponding request
				if strconv.Itoa(conn.attempt) == msg_seq {
					conn.recvCount += 1
					conn.leaders += count

					if leaderId != 0 {
						conn.leaderId = leaderId
					}
				}
			}
		}
	}
}
