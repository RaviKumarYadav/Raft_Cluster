package main

import (
	"encoding/json"
	"fmt"
	cluster "github.com/RaviKumarYadav/Raft/cluster"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
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
	Term() int
	Election_Timeout() int
	State() int
	Start() bool
	IsLeader() bool
}

type Machine struct {
	server           cluster.Server
	term             int
	election_timeout time.Duration
	// Leader,Follower,Candidate
	state           int
	machine_channel chan int
	voted           bool
	leaderId        int
}

func (m Machine) IsLeader() bool {

	if m.server.Pid() == m.leaderId {
		return true
	}

	return false

}

func (m Machine) Term() int {
	return m.term
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

// Returns Term (based on its Server_Id)
func (m Machine) readData(myServerId int, serverStateFile string) int {

	file, e := ioutil.ReadFile("./" + serverStateFile)

	if e != nil {
		fmt.Printf("File error: %v\n", e)
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

func NewMachine(myServerId int, serverStateFile string, configFile string, minTimeout int, maxTimeout int) Machine {

	machine := Machine{}

	tempTerm := machine.readData(myServerId, serverStateFile)

	if tempTerm == -1 {
		fmt.Println("Error!!! Term is '-1'. ")
		//return
	}

	machine.server = cluster.NewServer(myServerId, configFile)
	machine.term = tempTerm
	machine.election_timeout = time.Duration(minTimeout+rand.Intn(maxTimeout-minTimeout)) * time.Second
	machine.state = Follower
	machine.voted = false
	machine.machine_channel = make(chan int)

	return machine
}

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

// Processes the "append entries" request.
// Returns Term , Success
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

// Processes the "RequestVote" request.
// Returns Term , Success
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

func LeaderToAppendEntriesResponse(msg *cluster.Envelope, m *Machine) {

	fmt.Println("\nReceived HeartBeat Response from Server", msg.SenderId)

	return
}

func candidateLoop(m *Machine) {

	fmt.Println("\nIn Candidate Loop... Server", m.server.Pid())

	electionTimeout := m.Election_Timeout()
	ticker := time.NewTicker(electionTimeout)

	// So the Candidate has voted for itself and cannot vote for any one else
	voteCount := 1
	m.voted = true
	m.term = m.term + 1

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
			// It has to return after becoming a Candidate ,
			// so that few steps like 'increment of Term' , 'setting Timeout' could happen
			return

		}

	}
}

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

// Processes the "append entries" request.
// Returns Term , Success
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

// Processes the "RequestVote" request.
// Returns Term , Success
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

		}

	}
}

// Processes the "append entries" request.
// Returns Term , Success
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

// Processes the "RequestVote" request.
// Returns Term , Success
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

		m.server.Outbox() <- replyMsg
		m.voted = true

		return m.Term(), true

	}

	return m.Term(), false
}

func main() {

	fmt.Println("Starting Main.\n")

	myServerId, _ := strconv.Atoi(os.Args[1])

	machine := NewMachine(myServerId, "serverState.json", "config.json", 10, 12)

	fmt.Println("\nServer Part of Machine is Initialized")

	fmt.Println("\nStarting Receive Routine...")
	go cluster.ReceiveMessage(&machine.server)
	fmt.Println("\nStarting Send Routine...")
	go cluster.SendMessage(&machine.server)

	fmt.Println("\nElection TimeOut", machine.Election_Timeout())

	Start(&machine)

	fmt.Println("\nMain is Ended")
}
