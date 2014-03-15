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
	//// underlying program (invkoed by exec.Command) being executed.
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

type Connection struct {
	attempt       int
	sendCount     int
	recvCount     int
	leaders       int
	leaderId      int
	selfPort      int
	KillConn      chan string
	stopGoRoutine chan string
}

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
	//cmd.Wait()

	// Checking for the SERVER to be closed
	for {
		code := <-conn.KillConn
		kill_code, _ := strconv.Atoi(code)

		if kill_code == -1 || kill_code == sId {
			cmd.Process.Kill()
		}
	}

}

func TestLeaderWith_N_KillServers(t *testing.T) {

	max := 4

	fmt.Println("Test3 :- Testing with ", max-1, " Leader killed")

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

		time.Sleep(4000 * time.Millisecond)

		// Send Message and Receive the Status
		for _, sock := range sendSockets {
			sock.Send(strconv.Itoa(conn.attempt), zmq.DONTWAIT)
			conn.sendCount += 1
		}

		time.Sleep(500 * time.Millisecond)

		// Find who is leader and then kill it and the check after few seconds
		fmt.Println("\nDetails conn.sendCount :- ", conn.sendCount, " , conn.totalCount :- ", conn.recvCount, " , conn.leaders :- ", conn.leaders, " , conn.leaderId:- ", conn.leaderId)

		if i != max-1 {
			fmt.Println("\nCurrent Leader is Server", conn.leaderId)
			fmt.Println("Killing Server", conn.leaderId)
			for i := 0; i < noOfServers; i++ {
				conn.KillConn <- strconv.Itoa(conn.leaderId)
			}
		}
	}

	fmt.Println("Killing All Servers")
	// Kill all the servers
	for i := 0; i < noOfServers; i++ {
		conn.KillConn <- "-1"
	}

	//time.Sleep(500*time.Millisecond)
	//close(conn.KillConn)
	//fmt.Println("Before")
	//conn.stopGoRoutine <- "stop"
	//fmt.Println("After")

	fmt.Println("\nAll Servers got killed...\n")

	//fmt.Println("\nDetails conn.sendCount :- ",conn.sendCount," , conn.totalCount :- ",conn.recvCount," , conn.leaders :- ",conn.leaders," , conn.leaderId:- ",conn.leaderId)

	if conn.leaders != 1 {
		t.Errorf("totalLeaders - %v ", conn.leaders)
	}

	fmt.Printf("\n")

}

func receiveMessage(recSockets *zmq.Socket, conn *Connection) {

	for {
		//		var msg string

		select {
		case _, _ = <-conn.stopGoRoutine:

			fmt.Println("Stopping Go Routine")

			//close(conn.stopGoRoutine)
			return

		default:

			msg, _ := recSockets.Recv(0)
			fmt.Println("Msg Recv :- ", msg)

			//msg_seq:IsLeader:LeaderId:SelfId
			msg_array := strings.Split(msg, ":")

			msg_seq := msg_array[0]
			isLeader := msg_array[1]
			leaderId, _ := strconv.Atoi(msg_array[2])
			//senderId := msg_array[3]

			count, _ := strconv.Atoi(isLeader)

			// Added the LeaderCount if the response belongs to corresponding request
			if strconv.Itoa(conn.attempt) == msg_seq {
				conn.recvCount += 1
				conn.leaders += count

				if leaderId != 0 {
					conn.leaderId = leaderId
					fmt.Println("Leader Id :- ", conn.leaderId)
				}
			}
		}
	}
}
