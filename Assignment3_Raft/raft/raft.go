package main

import (
	"encoding/json"
	"fmt"
	cluster "github.com/RaviKumarYadav/Raft/cluster"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	RequestVote   int = 0
	AppendEntries int = 1
)

const (
	Leader    int = 0
	Follower  int = 1
	Candidate int = 2
	Stopped   int = 3
)

const (
	Packet_Received int = 0
	Forced_Stop     int = 1
)

type Raft interface {
	Term() int64
	Leader() int

	// Mailbox for state machine layer above to send commands of any
	// kind, and to have them replicated by raft.  If the server is not
	// the leader, the message will be silently dropped.
	Outbox() chan<- interface{}

	//Mailbox for state machine layer above to receive commands. These
	//are guaranteed to have been replicated on a majority
	Inbox() <-chan *LogEntry

	//Remove items from 0 .. index (inclusive), and reclaim disk
	//space. This is a hint, and there's no guarantee of immediacy since
	//there may be some servers that are lagging behind).
	DiscardUpto(index int64)

	Election_Timeout() int
	State() int
	Start() bool
	IsLeader() bool
}

// Identifies an entry in the log
type LogEntry struct {
	// An index into an abstract 2^64 size array
	Index int64
	Term  int64

	// The data that was supplied to raft's inbox
	Data interface{}
}

type Machine struct {
	server           cluster.Server
	term             int
	election_timeout time.Duration

	// Leader,Follower,Candidate
	state int

	// machine_channel can be used to pass Stop-Signal to stop the machine
	machine_channel chan int

	voted    bool
	leaderId int
}

/*************************************************
	Tester Methods
*************************************************/

type Testerstruct struct {
	SendSocket *zmq.Socket
	RecvSocket *zmq.Socket
}

func (t *Testerstruct) TesterInit(m *Machine, testerPort string) {

	ownPort := (strings.Split(m.server.ServerAddress(), ":"))[2]
	receivePort, _ := strconv.Atoi(ownPort)
	strRecvPort := strconv.Itoa(receivePort + 1)

	// Receive Port is initialized
	t.RecvSocket, _ = zmq.NewSocket(zmq.PULL)
	t.RecvSocket.Bind("tcp://*:" + strRecvPort)

	// Send Port is initialized
	t.SendSocket, _ = zmq.NewSocket(zmq.PUSH)
	t.SendSocket.Connect("tcp://localhost:" + testerPort)
}

func (t *Testerstruct) TesterRecv(m *Machine) {

	for {
		msg_recv, _ := t.RecvSocket.Recv(0)
		t.TesterSend(m, msg_recv)
	}

}

func (t *Testerstruct) TesterSend(m *Machine, recv_msg string) {
	//msg_seq:IsLeader:LeaderId:SelfId
	// msg --> msg_seq_no. only
	msg := recv_msg

	if m.IsLeader() == true {
		msg += ":1"
	} else if m.IsLeader() == false {
		msg += ":0"
	}

	msg += ":" + strconv.Itoa(m.leaderId)
	msg += ":" + strconv.Itoa(m.server.Pid())

	t.SendSocket.Send(msg, zmq.DONTWAIT)
}

/*************************************************
	Tester Methods Closed
*************************************************/

func (m Machine) IsLeader() bool {

	if m.server.Pid() == m.leaderId {
		return true
	}

	return false

}

func (m Machine) Term() int {
	return m.term
}

func (m Machine) Leader() int {
	return m.leaderId
}

func (m Machine) Election_Timeout() time.Duration {
	return m.election_timeout
}

func (m Machine) State() int {
	return m.state
}

type jsontype struct {
	ServerState []ServerState
}

type ServerState struct {
	Server_Id int
	Term      int
}

/*************************************************
	Returns Term (based on its Server_Id)
*************************************************/
func (m Machine) readData(myServerId int, serverStateFile string) int {

	file, e := ioutil.ReadFile("./" + serverStateFile)

	if e != nil {
		fmt.Printf("Raft File error: %v\n", e)
		os.Exit(1)
	}

	var jsonobject jsontype
	json.Unmarshal(file, &jsonobject)

	for _, value := range jsonobject.ServerState {
		if myServerId == value.Server_Id {

			return value.Term

		}
	}

	// If no Term is defined for Server
	return -1
}

/****************************************************************************************************
	Creates a new instance of Machine (which contains Server object) based on ServerId passed 
	and its corresponding information stored in Config.json , ServerState.json file
****************************************************************************************************/
func NewMachine(myServerId int, serverStateFile string, configFile string, minTimeout int, maxTimeout int) Machine {

	machine := Machine{}

	tempTerm := machine.readData(myServerId, serverStateFile)

	if tempTerm == -1 {
		fmt.Println("Error!!! Term is '-1'. ")
		//return
	}

	machine.server = cluster.NewServer(myServerId, configFile)

	machine.term = tempTerm

	// Random no. generation based SEED which is their own ServerId 
	seed := rand.NewSource(int64(2*myServerId - 1))
	r1 := rand.New(seed)
	machine.election_timeout = time.Duration(minTimeout+r1.Intn(maxTimeout-minTimeout)) * time.Millisecond

	machine.state = Follower
	machine.voted = false
	machine.machine_channel = make(chan int)

	return machine
}

/*********************************************************************
	Starting Point of Server ,
	It controls the switching of Server among different phases
*********************************************************************/
func Start(m *Machine) {

	fmt.Println("\nIn Start...")

	for m.state != Stopped {

		state := m.State()

		switch state {
		case Follower:
			followerLoop(m)
		case Candidate:
			candidateLoop(m)
		case Leader:
			leaderLoop(m)
		}
	}
}

/*********************************************************************
	Loops in Leader Phase and Responds to requests as Leader
*********************************************************************/
func leaderLoop(m *Machine) {

	fmt.Println("\nIn Leader Loop...")

	electionTimeout := m.Election_Timeout()
	ticker := time.NewTimer(electionTimeout / 2)

	// As soon as this machine becomes a leader , it should do the following tasks :-
	// 1.	Set 'leaderId' to its own 'serverId'
	// 2.	Set its own state to 'Leader'
	// 3.	Send Heartbeat Signal to all (so BROADCAST it)
	m.leaderId = m.server.Pid()
	m.state = Leader
	m.voted = true

	msg := &cluster.Envelope{}

	msg.Pid = -1
	msg.MsgType = cluster.AppendEntriesRequest
	msg.SenderId = m.server.Pid()
	msg.Term = m.Term()
	msg.Msg = "Broadcast Message by Leader"

	fmt.Println("\n---------------------------------------")
	fmt.Println("Sending HeartBeat Signal to all Servers")
	fmt.Println("---------------------------------------")

	m.server.Outbox() <- msg

	for m.State() == Leader {

		select {
		case value := <-m.machine_channel:

			if value == Forced_Stop {
				m.state = Stopped
				return
			}

		case msg_received := <-m.server.Inbox():

			// Gather all Vote Responses from others and Wish to become Leader

			switch msg_received.MsgType {

			case cluster.AppendEntriesRequest:

				LeaderToAppendEntriesRequest(msg_received, m)

			case cluster.AppendEntriesResponse:

				LeaderToAppendEntriesResponse(msg_received, m)

			case cluster.RequestVoteRequest:
				// Here the request is coming from a Candidate as Request Type is RequestVote
				LeaderToRequestVoteRequest(msg_received, m)
			}

		case <-ticker.C:
			// Wait for "How much time is left for timeout/2 and then send a heartbeat message"

			ticker = time.NewTimer(electionTimeout / 2)

			sendMsg := &cluster.Envelope{}

			sendMsg.Pid = -1 // Replying to the Sender who sent request
			sendMsg.MsgType = cluster.AppendEntriesRequest
			sendMsg.SenderId = m.server.Pid()

			sendMsg.Msg = "Heartbeat Request by Leader"
			sendMsg.Term = m.Term()

			m.server.Outbox() <- sendMsg

			fmt.Println("\n---------------------------------------")
			fmt.Println("Sending HeartBeat Signal to all Servers")
			fmt.Println("---------------------------------------")

		}
	}
}

/*********************************************************************
	Processes the "AppendEntries" request.
	Returns Term , Success
*********************************************************************/
func LeaderToAppendEntriesRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.Term() >= msg.Term {
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()

		replyMsg.Msg = "Leaders (" + strconv.Itoa(m.server.Pid()) + ") Response to AE by Server" + strconv.Itoa(msg.SenderId)

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() < msg.Term {
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()

		replyMsg.Msg = "In LeaderToAppendEntriesRequest myTerm is Smaller of Server" + strconv.Itoa(m.server.Pid())

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		// discover new leader when candidate
		// save leader name when follower
		m.leaderId = msg.SenderId

		m.server.Outbox() <- replyMsg

		return m.Term(), true
	}

	return m.Term(), false
}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func LeaderToRequestVoteRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.Term() >= msg.Term {
		// Either we can discard packet or sent our own 'Term' so that Candidate can 
		// updates its own term and convert to Follower
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()
		replyMsg.Voted = cluster.VotedNo

		replyMsg.Msg = "In LeaderToRequestVoteRequest 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() < msg.Term {
		// New Election have started , so have to upgrade your Term and vote

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes
		replyMsg.Term = msg.Term
		replyMsg.Msg = "In LeaderToRequestVoteRequest 'myTerm is Smaller' of Server" + strconv.Itoa(m.server.Pid())

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower

		m.server.Outbox() <- replyMsg
		m.voted = true

		return m.Term(), true

	}

	return m.Term(), false
}

/*********************************************************************
	Sending "AppendEntryResponse"
	Returns Term , Success
*********************************************************************/
func LeaderToAppendEntriesResponse(msg *cluster.Envelope, m *Machine) {

	fmt.Println("\nReceived HeartBeat Response from Server", msg.SenderId)

	return
}

/*********************************************************************
	Working as Candidate
*********************************************************************/
func candidateLoop(m *Machine) {

	fmt.Println("\nIn Candidate Loop... Server", m.server.Pid())

	electionTimeout := m.Election_Timeout()
	ticker := time.NewTicker(electionTimeout)

	// So the Candidate has voted for itself and cannot vote for any one else
	voteCount := 1
	m.voted = true
	m.term = m.term + 1
	m.leaderId = 0

	// RequestVote Message to all other Servers in cluster
	reqVote := &cluster.Envelope{}

	reqVote.Pid = -1 // Replying to the Sender who sent request
	reqVote.MsgType = cluster.RequestVoteRequest
	reqVote.SenderId = m.server.Pid()
	reqVote.Term = m.Term()
	reqVote.Msg = "RequestVote by Candidate."
	m.server.Outbox() <- reqVote

	fmt.Println("\nFighting for Term", m.Term())
	fmt.Println("\nRequest Vote sent to All ... by Candidate", m.server.Pid())

	// Sense for Votes Gathered in Election
	for m.State() == Candidate {

		select {
		case value := <-m.machine_channel:

			if value == Forced_Stop {
				m.state = Stopped
				return
			}

		case msg_received := <-m.server.Inbox():

			// Gather all Vote Responses from others and Wish to become Leader
			fmt.Println("Received Vote from Server", msg_received.SenderId, " as ", msg_received.Voted)

			switch msg_received.MsgType {
			case cluster.RequestVoteResponse:
				votesGathered, GotMajority := CandidateToRequestVoteResponse(msg_received, voteCount, m)

				voteCount = votesGathered

				if GotMajority {
					m.state = Leader
					m.leaderId = m.server.Pid()
					return
				}

			case cluster.AppendEntriesRequest:

				CandidateToAppendEntries(msg_received, m)

			case cluster.RequestVoteRequest:

				// Here the request is coming from a Candidate as Request Type is RequestVote
				CandidateToRequestVote(msg_received, m)
			}
		case <-ticker.C:
			fmt.Println("\nTimeout occurred in Candidate Server", m.server.Pid())
			m.state = Candidate
			m.leaderId = 0
			// It has to return after becoming a Candidate ,
			// so that few steps like 'increment of Term' , 'setting Timeout' could happen
			return

		}

	}
}

/*********************************************************************
	Responding to "RequestVoteRequest"
*********************************************************************/
func CandidateToRequestVoteResponse(msg *cluster.Envelope, voteCount int, m *Machine) (int, bool) {

	totalServer := len(m.server.Peers()) + 1
	majority := (totalServer / 2) + 1

	if (m.Term() == msg.Term) && (msg.Voted == cluster.VotedYes) {

		voteCount++
		if voteCount >= majority {
			return voteCount, true
		}
	}

	return voteCount, false
}

/*********************************************************************
	Processes the "append entries" request.
	Returns Term , Success
*********************************************************************/
func CandidateToAppendEntries(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.Term() >= msg.Term {
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()

		replyMsg.Msg = "In CandidateToAppendEntries 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() < msg.Term {

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()

		replyMsg.Msg = "In CandidateToAppendEntries 'myTerm is Smaller' of Server" + strconv.Itoa(m.server.Pid())

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		// discover new leader when candidate
		// save leader name when follower
		m.leaderId = msg.SenderId

		m.server.Outbox() <- replyMsg

		return m.Term(), true
	}

	return m.Term(), false
}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func CandidateToRequestVote(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.Term() >= msg.Term {
		// Either we can discard packet or sent our own 'Term' so that Candidate can 
		// updates its own term and convert to Follower
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()
		replyMsg.Voted = cluster.VotedNo

		replyMsg.Msg = "In CandidateToRequestVote 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() == msg.Term {

		if m.voted == false {

			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.RequestVoteResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Voted = cluster.VotedYes
			replyMsg.Msg = "In CandidateToRequestVote 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())
			replyMsg.Term = msg.Term
			// change state to follower
			m.state = Follower
			// discover new leader when candidate
			// Dont save leader name as Its a VoteRequest so its not a leader anymore
			//m.leaderId = msg.SenderId

			m.server.Outbox() <- replyMsg
			m.voted = true

			return m.Term(), true

		} else {

			// Need not to do anything as you have already voted for someone.
			fmt.Println("Just Chill !!! Vote already casted...")
		}

	} else if m.Term() < msg.Term {

		// New Election have started , so have to upgrade your Term and vote

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes
		replyMsg.Msg = "In CandidateToRequestVote 'myTerm is Smaller' of Server" + strconv.Itoa(m.server.Pid())
		replyMsg.Term = msg.Term

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		// discover new leader when candidate
		// Dont save leader name as Its a VoteRequest so its not a leader anymore
		//m.leaderId = msg.SenderId

		m.server.Outbox() <- replyMsg
		m.voted = true

		return m.Term(), true

	}

	return m.Term(), false
}

/*********************************************************************
	Working like a Follower
*********************************************************************/
func followerLoop(m *Machine) {

	fmt.Println("\nIn Follower Loop...")

	electionTimeout := m.Election_Timeout()

	for Follower == m.State() {

		// Each time timer will be set (equal to Timeout)
		tickerFollower := time.NewTicker(electionTimeout)

		select {

		case value := <-m.machine_channel:

			if value == Forced_Stop {
				m.state = Stopped
				return
			}

		case msg_received := <-m.server.Inbox():

			switch msg_received.MsgType {

			case cluster.AppendEntriesRequest:
				// If heartbeats get too close to the election timeout then send an event.
				//elapsedTime := time.Now().Sub(since)
				FollowerToAppendEntriesRequest(msg_received, m)
			case cluster.RequestVoteRequest:
				// Here the request is coming from a Candidate as Request Type is RequestVote
				FollowerToRequestVoteRequest(msg_received, m)
			}

		case <-tickerFollower.C:
			fmt.Println("\nTimeout occurred in Server", m.server.Pid())
			m.state = Candidate
			m.leaderId = 0

		}

	}
}

/*********************************************************************
	Processes the "append entries" request.
	Returns Term , Success
*********************************************************************/
func FollowerToAppendEntriesRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	fmt.Println("\nReplying to HeartBeat of Leader i.e. Server", msg.SenderId)

	if m.Term() > msg.Term {
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()

		replyMsg.Msg = "In FollowerToAppendEntries 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() == msg.Term {

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()

		replyMsg.Msg = "In FollowerToAppendEntries 'myTerm is Equal' of Server" + strconv.Itoa(m.server.Pid())

		// change state to follower
		m.state = Follower
		// discover new leader when candidate
		// save leader name when follower
		m.leaderId = msg.SenderId

		m.server.Outbox() <- replyMsg

		return m.Term(), true

	} else if m.Term() < msg.Term {

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()

		replyMsg.Msg = "In FollowerToAppendEntries 'myTerm is Smaller' of Server" + strconv.Itoa(m.server.Pid())

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		// discover new leader when candidate
		// save leader name when follower
		m.leaderId = msg.SenderId

		m.server.Outbox() <- replyMsg

		return m.Term(), true
	}

	return m.Term(), false
}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func FollowerToRequestVoteRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.Term() > msg.Term {
		// Either we can discard packet or sent our own 'Term' so that Candidate can 
		// updates its own term and convert to Follower
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()
		replyMsg.Voted = cluster.VotedNo

		replyMsg.Msg = "In FollowerToRequestVote 'myTerm is Greater' of Server" + strconv.Itoa(m.server.Pid())

		m.server.Outbox() <- replyMsg

		return m.Term(), false

	} else if m.Term() == msg.Term {

		if m.voted == false {

			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.RequestVoteResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Voted = cluster.VotedYes
			replyMsg.Msg = "In FollowerToRequestVote 'myTerm is equal' of Server" + strconv.Itoa(m.server.Pid())
			replyMsg.Term = msg.Term

			// change state to follower
			m.state = Follower
			m.leaderId = 0

			m.server.Outbox() <- replyMsg
			m.voted = true

			return m.Term(), true

		}

	} else if m.Term() < msg.Term {
		// New Election have started , so have to upgrade your Term and vote

		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes
		replyMsg.Msg = "In FollowerToRequestVote 'myTerm is Smaller' of Server" + strconv.Itoa(m.server.Pid())
		replyMsg.Term = msg.Term

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		m.leaderId = 0

		m.server.Outbox() <- replyMsg
		m.voted = true

		return m.Term(), true

	}

	return m.Term(), false
}

/******************************************************************************************
	Main() method , starts the proceedings by sending the Server in Follower phase
******************************************************************************************/
func main() {

	myServerId, _ := strconv.Atoi(os.Args[1])
	serverStateFile := os.Args[2]
	configFile := os.Args[3]
	minTimeOut, _ := strconv.Atoi(os.Args[4])
	maxTimeOut, _ := strconv.Atoi(os.Args[5])

	fmt.Println("Starting Main", myServerId)

	machine := NewMachine(myServerId, serverStateFile, configFile, minTimeOut, maxTimeOut)

	//***********For Tester Purpose Only***********
	testerPort := os.Args[6]

	var t Testerstruct

	t.TesterInit(&machine, testerPort)
	go t.TesterRecv(&machine)

	//*********Tester Methods Closed***************
	go cluster.ReceiveMessage(&machine.server)

	go cluster.SendMessage(&machine.server)

	fmt.Println("\nElection TimeOut", machine.Election_Timeout())

	Start(&machine)

}
