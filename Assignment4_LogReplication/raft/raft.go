package main

import (
	"encoding/gob"
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

/**************************************************
	Tester Methods
**************************************************/

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

/**************************************************
	Tester Methods Closed
**************************************************/

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

/**************************************************
	Returns Term (based on its Server_Id)
**************************************************/
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

/*****************************************************************************************************
	Creates a new instance of Machine ( It contains Server instance) based on ServerId passed 
	and its corresponding information stored in Config.json , ServerStateFile file
*****************************************************************************************************/
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

	fmt.Println("\n---------------------------------------")
	fmt.Println("Sending HeartBeat Signal to all Servers")
	fmt.Println("---------------------------------------")

	msg := &cluster.Envelope{}

	msg.Pid = -1
	msg.MsgType = cluster.AppendEntriesRequest
	msg.SenderId = m.server.Pid()
	msg.Term = m.Term()

	prevLTerm := 0

	if m.LastLogIndex != -1 {
		prevLTerm = m.Log[m.LastLogIndex].Term
	}

	AERequestStruct := cluster.AppendEntriesRequestStruct{PrevLogIndex: m.LastLogIndex, PrevLogTerm: prevLTerm, Entries: make([]cluster.LogEntry, 0), LeaderCommit: m.CommitIndex}

	msg.Msg = AERequestStruct

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

			for _, Sid := range m.server.Peers() {
				m.NextIndex[Sid] = m.MatchIndex[Sid] + 1
			}

			for _, Sid := range m.server.Peers() {
				msg := &cluster.Envelope{}
				msg.Pid = Sid
				msg.MsgType = cluster.AppendEntriesRequest
				msg.SenderId = m.server.Pid()
				msg.Term = m.Term()

				lastTerm := 0

				if m.MatchIndex[Sid] != -1 {
					lastTerm = m.Log[m.MatchIndex[Sid]].Term
				}

				msg.Msg = cluster.AppendEntriesRequestStruct{PrevLogIndex: m.MatchIndex[Sid], PrevLogTerm: lastTerm, Entries: m.Log[m.NextIndex[Sid] : m.LastLogIndex+1], LeaderCommit: m.CommitIndex}

				m.server.Outbox() <- msg
				fmt.Println("For Server ", Sid, " , Raft Inbox :- ", msg.Msg)
			}

			fmt.Println("\n---------------------------------------")
			fmt.Println("Sending HeartBeat Signal to all Servers")
			fmt.Println("---------------------------------------")

		}

		ticker = time.NewTimer(electionTimeout / 2)
	}
}

/*********************************************************************
	Processes the "AppendEntries" request.
	Returns Term , Success
*********************************************************************/
func LeaderToAppendEntriesRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	if m.server.Pid() == msg.SenderId {
		// My own AE_Request , "How foolish I am ?" :) 
		// Neglect this.
	}

	req := msg.Msg.(cluster.AppendEntriesRequestStruct)

	if m.Term() > msg.Term {

		// Do Not Reply

	} else if m.Term() <= msg.Term {

		m.term = msg.Term

		localLastLogTerm := 0

		if m.LastLogIndex != -1 {
			localLastLogTerm = m.Log[m.LastLogIndex].Term
		}

		if localLastLogTerm < msg.Term {
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.AppendEntriesResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Term = m.Term()

			replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
			m.server.Outbox() <- replyMsg

			m.state = Follower
			m.leaderId = msg.SenderId
		} else if localLastLogTerm == msg.Term {

			// Check whether "req.PrevLogIndex" exists or not
			if m.LastLogIndex <= req.PrevLogIndex {
				// False : Log is not up-to-date , so send own Status
				replyMsg := &cluster.Envelope{}

				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.AppendEntriesResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Term = m.Term()

				replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
				m.server.Outbox() <- replyMsg

				m.state = Follower
				m.leaderId = msg.SenderId
			}

		}
	}

	return m.Term(), false

	return m.Term(), false

}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func LeaderToRequestVoteRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	req := msg.Msg.(cluster.RequestVoteRequestStruct)

	if m.Term() < msg.Term {
		// Then Vote
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes

		replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

		replyMsg.Term = msg.Term

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		m.leaderId = 0

		m.server.Outbox() <- replyMsg
		m.voted = true
	} else if m.Term() == msg.Term {
		// Before Vote , Check other details

		localLogLastIndex := m.LastLogIndex
		localLogLastTerm := 0

		if localLogLastIndex != -1 {
			localLogLastTerm = m.Log[localLogLastIndex].Term
		}

		if localLogLastTerm < req.PrevLogTerm { // We have latest entry with equal Term so we check for longer entry or logIndex
			// Vote
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.RequestVoteResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Voted = cluster.VotedYes

			replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

			replyMsg.Term = msg.Term

			//Update term and leader.
			m.term = msg.Term
			// change state to follower
			m.state = Follower
			m.leaderId = 0

			m.server.Outbox() <- replyMsg
			m.voted = true

		} else if localLogLastTerm == req.PrevLogTerm {

			if localLogLastIndex < req.PrevLogIndex {
				// Vote
				replyMsg := &cluster.Envelope{}

				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.RequestVoteResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Voted = cluster.VotedYes

				replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

				replyMsg.Term = msg.Term

				//Update term and leader.
				m.term = msg.Term
				// change state to follower
				m.state = Follower
				m.leaderId = 0

				m.server.Outbox() <- replyMsg
				m.voted = true
			}
		}
	}

	return m.Term(), false

}

/*********************************************************************
	Sending "AppendEntryResponse"
	Returns Term , Success
*********************************************************************/
func LeaderToAppendEntriesResponse(msg *cluster.Envelope, m *Machine) {

	res := msg.Msg.(cluster.AppendEntriesResponseStruct)

	fmt.Println("\nReceived HeartBeat Response from Server", msg.SenderId, " as :- ", res)

	m.MatchIndex[msg.SenderId] = res.LastLogEntry
	m.FollowerTerm[msg.SenderId] = msg.Term

	// CommitIndex will be moved forward based on the LastLogIndex of Majority of Servers in Cluster (including it's own commitIndex)
	commitIndexToFreq := make(map[int64]int)
	commitIndexToFreq[m.LastLogIndex] = 1

	// Find Log Status of all Servers
	for sId, value := range m.MatchIndex {

		_, ok := commitIndexToFreq[value]

		if ok == true && m.Term() == m.FollowerTerm[sId] {
			commitIndexToFreq[value] += 1
		} else if ok == false && m.Term() == m.FollowerTerm[sId] {
			commitIndexToFreq[value] = 1
		}
	}

	// Array to hold all distinct Commit Indices
	var indices []int64

	for key, _ := range commitIndexToFreq {
		indices = append(indices, key)
	}

	// Sort (Decreasing Order) based on CommitIndex
	for i := 0; i < len(indices)-1; i++ {

		maxValue := indices[i]
		maxIndex := i

		for j := i; j < len(indices); j++ {

			if indices[j] > maxValue {
				maxValue = indices[j]
				maxIndex = j
			}
		}

		indices[maxIndex] = indices[i]
		indices[i] = maxValue
	}

	//Check Each CommitIndex (from high to low) and evalute its count whether it qualifies majority for updating old commitIndex
	fmt.Println("Map CommitIndex to Count :- ", commitIndexToFreq)
	for _, key := range indices {

		if commitIndexToFreq[key] >= (len(m.server.Peers())/2+1) && key > m.CommitIndex {
			m.CommitIndex = key
			fmt.Println("Commit Index :- ", m.CommitIndex)
			break
		} else if key < m.CommitIndex {
			break
		}
	}

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

	var lastLIndex int64 = -1
	lastLTerm := 0

	if m.LastLogIndex != -1 {
		lastLIndex = m.LastLogIndex
		lastLTerm = m.Log[m.LastLogIndex].Term
	}

	reqVoteRequestStruct := cluster.RequestVoteRequestStruct{PrevLogIndex: lastLIndex, PrevLogTerm: lastLTerm}

	reqVote.Msg = reqVoteRequestStruct

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
			fmt.Println("Received Vote of type ", msg_received.MsgType, " from Server", msg_received.SenderId, " as ", msg_received.Voted)

			switch msg_received.MsgType {
			case cluster.RequestVoteResponse:
				votesGathered, GotMajority := CandidateToRequestVoteResponse(msg_received, &voteCount, m)

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

		ticker = time.NewTicker(electionTimeout)
	}
}

/*********************************************************************
	Responding to "RequestVoteRequest"
*********************************************************************/
func CandidateToRequestVoteResponse(msg *cluster.Envelope, voteCount *int, m *Machine) (int, bool) {

	totalServer := len(m.server.Peers()) + 1
	majority := (totalServer / 2) + 1

	if (m.Term() == msg.Term) && (msg.Voted == cluster.VotedYes) {

		*voteCount = *voteCount + 1
		fmt.Println("Vote Count :- ", *voteCount, " , Majority", majority, " , Servers :- ", totalServer)

		if *voteCount >= majority {
			return *voteCount, true
		}
	}

	return *voteCount, false
}

/*********************************************************************
	Processes the "append entries" request.
	Returns Term , Success
*********************************************************************/
func CandidateToAppendEntries(msg *cluster.Envelope, m *Machine) (int, bool) {

	req := msg.Msg.(cluster.AppendEntriesRequestStruct)

	if m.Term() > msg.Term {

		// Do Not Reply

	} else if m.Term() <= msg.Term {

		m.term = msg.Term

		localLastLogTerm := 0

		if m.LastLogIndex != -1 {
			localLastLogTerm = m.Log[m.LastLogIndex].Term
		}

		if localLastLogTerm < msg.Term {
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.AppendEntriesResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Term = m.Term()

			replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
			m.server.Outbox() <- replyMsg

			m.state = Follower
			m.leaderId = msg.SenderId
		} else if localLastLogTerm == msg.Term {

			// Check whether "req.PrevLogIndex" exists or not
			if m.LastLogIndex <= req.PrevLogIndex {
				// False : Log is not up-to-date , so send own Status
				replyMsg := &cluster.Envelope{}

				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.AppendEntriesResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Term = m.Term()

				replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
				m.server.Outbox() <- replyMsg

				m.state = Follower
				m.leaderId = msg.SenderId
			}

		}
	}

	return m.Term(), false
}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func CandidateToRequestVote(msg *cluster.Envelope, m *Machine) (int, bool) {

	req := msg.Msg.(cluster.RequestVoteRequestStruct)

	if m.Term() < msg.Term {
		// Then Vote
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes

		replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

		replyMsg.Term = msg.Term

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		m.leaderId = 0

		m.server.Outbox() <- replyMsg
		m.voted = true
	} else if m.Term() == msg.Term {
		// Before Vote , Check other details

		localLogLastIndex := m.LastLogIndex
		localLogLastTerm := 0

		if localLogLastIndex != -1 {
			localLogLastTerm = m.Log[localLogLastIndex].Term
		}

		if localLogLastTerm < req.PrevLogTerm { // We have latest entry with equal Term so we check for longer entry or logIndex
			// Vote
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.RequestVoteResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Voted = cluster.VotedYes

			replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

			replyMsg.Term = msg.Term

			//Update term and leader.
			m.term = msg.Term
			// change state to follower
			m.state = Follower
			m.leaderId = 0

			m.server.Outbox() <- replyMsg
			m.voted = true

		} else if localLogLastTerm == req.PrevLogTerm {

			if localLogLastIndex < req.PrevLogIndex {
				// Vote
				replyMsg := &cluster.Envelope{}

				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.RequestVoteResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Voted = cluster.VotedYes

				replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

				replyMsg.Term = msg.Term

				//Update term and leader.
				m.term = msg.Term
				// change state to follower
				m.state = Follower
				m.leaderId = 0

				m.server.Outbox() <- replyMsg
				m.voted = true
			}
		}
	}

	return m.Term(), false
}

/*********************************************************************
	Working like a Follower
*********************************************************************/
func followerLoop(m *Machine) {

	fmt.Println("\nIn Follower Loop...")

	electionTimeout := m.Election_Timeout()

	// Each time timer will be set (equal to Timeout)
	tickerFollower := time.NewTicker(electionTimeout)

	for Follower == m.State() {

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

		tickerFollower = time.NewTicker(electionTimeout)
	}
}

/*********************************************************************
	Processes the "append entries" request.
	Returns Term , Success
*********************************************************************/
func FollowerToAppendEntriesRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	req := msg.Msg.(cluster.AppendEntriesRequestStruct)

	if m.Term() > msg.Term {
		// Reply False
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.AppendEntriesResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Term = m.Term()

		fmt.Println("LastLogIndex :- ", m.LastLogIndex, " , Location :- ", 1)
		replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}

		m.server.Outbox() <- replyMsg

	} else if m.Term() <= msg.Term {

		m.term = msg.Term

		localLastLogTerm := 0

		if m.LastLogIndex != -1 {
			localLastLogTerm = m.Log[m.LastLogIndex].Term
		}

		// Check whether "req.PrevLogIndex" exists or not
		if m.LastLogIndex < req.PrevLogIndex {
			// False : Log is not up-to-date , so send own Status
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.AppendEntriesResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Term = m.Term()
			fmt.Println("LastLogIndex :- ", m.LastLogIndex, " , Location :- ", 2)
			replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
			m.server.Outbox() <- replyMsg

		} else if m.LastLogIndex >= req.PrevLogIndex {
			fmt.Println("Own Last Index - ", m.LastLogIndex, " , Req Last Index :- ", req.PrevLogIndex)
			if localLastLogTerm == req.PrevLogTerm {
				// Great , Append if any logEntry is sent in message
				replyMsg := &cluster.Envelope{}
				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.AppendEntriesResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Term = m.Term()

				// Append Entries
				if len(req.Entries) > 0 {
					m.Log = append(m.Log, req.Entries...)
					m.LastLogIndex += int64(len(req.Entries))
				}

				if req.LeaderCommit > m.CommitIndex {
					// Server CommitIndex index = min (req.LeaderCommit,m.LastLogIndex)

					if req.LeaderCommit < m.LastLogIndex {
						m.CommitIndex = req.LeaderCommit
					} else {
						m.CommitIndex = m.LastLogIndex
					}
				}
				fmt.Println("LastLogIndex :- ", m.LastLogIndex, " , Commit Index :- ", m.CommitIndex, " Req.Commit Index :- ", req.LeaderCommit)
				replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: true, LastLogEntry: m.LastLogIndex}
				m.server.Outbox() <- replyMsg

			} else if localLastLogTerm != req.PrevLogTerm {
				// Remove current entry and all succeding entries also

				if req.PrevLogIndex != -1 {
					m.Log = m.Log[0:req.PrevLogIndex]
					m.LastLogIndex = req.PrevLogIndex - 1
				}

				replyMsg := &cluster.Envelope{}
				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.AppendEntriesResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Term = m.Term()
				fmt.Println("LastLogIndex :- ", m.LastLogIndex, " , Location :- ", 4)
				replyMsg.Msg = cluster.AppendEntriesResponseStruct{Success: false, LastLogEntry: m.LastLogIndex}
				m.server.Outbox() <- replyMsg
			}

		}

	}

	return m.Term(), false

}

/*********************************************************************
	Processes the "RequestVote" request.
	Returns Term , Success
*********************************************************************/
func FollowerToRequestVoteRequest(msg *cluster.Envelope, m *Machine) (int, bool) {

	req := msg.Msg.(cluster.RequestVoteRequestStruct)

	if m.Term() < msg.Term {
		// Then Vote
		replyMsg := &cluster.Envelope{}

		replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
		replyMsg.MsgType = cluster.RequestVoteResponse
		replyMsg.SenderId = m.server.Pid()
		replyMsg.Voted = cluster.VotedYes

		replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

		replyMsg.Term = msg.Term

		//Update term and leader.
		m.term = msg.Term
		// change state to follower
		m.state = Follower
		m.leaderId = 0

		m.server.Outbox() <- replyMsg
		m.voted = true
	} else if m.Term() == msg.Term {
		// Before Vote , Check other details

		localLogLastIndex := m.LastLogIndex
		localLogLastTerm := 0

		if localLogLastIndex != -1 {
			localLogLastTerm = m.Log[localLogLastIndex].Term
		}

		if localLogLastTerm < req.PrevLogTerm { // We have latest entry with equal Term so we check for longer entry or logIndex
			// Vote
			replyMsg := &cluster.Envelope{}

			replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
			replyMsg.MsgType = cluster.RequestVoteResponse
			replyMsg.SenderId = m.server.Pid()
			replyMsg.Voted = cluster.VotedYes

			replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

			replyMsg.Term = msg.Term

			//Update term and leader.
			m.term = msg.Term
			// change state to follower
			m.state = Follower
			m.leaderId = 0

			m.server.Outbox() <- replyMsg
			m.voted = true

		} else if localLogLastTerm == req.PrevLogTerm {

			if localLogLastIndex < req.PrevLogIndex {
				// Vote
				replyMsg := &cluster.Envelope{}

				replyMsg.Pid = msg.SenderId // Replying to the Sender who sent request
				replyMsg.MsgType = cluster.RequestVoteResponse
				replyMsg.SenderId = m.server.Pid()
				replyMsg.Voted = cluster.VotedYes

				replyMsg.Msg = cluster.RequestVoteResponseStruct{VoteGranted: true}

				replyMsg.Term = msg.Term

				//Update term and leader.
				m.term = msg.Term
				// change state to follower
				m.state = Follower
				m.leaderId = 0

				m.server.Outbox() <- replyMsg
				m.voted = true
			}
		}
	}

	return m.Term(), false
}

/******************************************************************************************
	Obtains Request from Client and then Push it on Outbox()
******************************************************************************************/
func (m *Machine) StartRaftInbox() {

	time.Sleep(5 * time.Second)

	i := int64(0)

	for {

		if m.IsLeader() == true {

			for _, Sid := range m.server.Peers() {
				m.NextIndex[Sid] = m.MatchIndex[Sid] + 1
			}

			entry := cluster.LogEntry{Index: i, Term: m.Term(), Data: strconv.Itoa(int(i)+1) + " Entry"}
			singleEntry := []cluster.LogEntry{entry}
			m.Log = append(m.Log, singleEntry...)
			m.LastLogIndex = int64(len(m.Log)) - 1
			fmt.Println("Entry no :- ", i, " Log Index :- ", m.LastLogIndex)

			for _, Sid := range m.server.Peers() {
				msg := &cluster.Envelope{}
				msg.Pid = Sid
				msg.MsgType = cluster.AppendEntriesRequest
				msg.SenderId = m.server.Pid()
				msg.Term = m.Term()

				lastTerm := 0

				if m.MatchIndex[Sid] != -1 {
					lastTerm = m.Log[m.MatchIndex[Sid]].Term
				}

				msg.Msg = cluster.AppendEntriesRequestStruct{PrevLogIndex: m.MatchIndex[Sid], PrevLogTerm: lastTerm, Entries: m.Log[m.NextIndex[Sid] : m.LastLogIndex+1], LeaderCommit: m.CommitIndex}

				m.server.Outbox() <- msg
				fmt.Println("Raft Inbox :- ", msg.Msg)
			}

			time.Sleep(2 * time.Second)
			i = i + 1
		} else {
			// Reply back with proper Leader details
			time.Sleep(2 * time.Second)

		}
	}
}

type Raft interface {
	Term() int
	Leader() int

	// Mailbox for state machine layer above to send commands of any
	// kind, and to have them replicated by raft.  If the server is not
	// the leader, the message will be silently dropped.
	Outbox() chan<- interface{}

	//Mailbox for state machine layer above to receive commands. These
	//are guaranteed to have been replicated on a majority
	Inbox() <-chan *cluster.LogEntry

	//Remove items from 0 .. index (inclusive), and reclaim disk
	//space. This is a hint, and there's no guarantee of immediacy since
	//there may be some servers that are lagging behind).
	DiscardUpto(index int64)

	Election_Timeout() int
	State() int
	//Start() 		bool
	IsLeader() bool
}

type Machine struct {
	server           cluster.Server
	term             int
	election_timeout time.Duration
	voted            bool
	leaderId         int
	state            int      // Leader,Follower,Candidate
	machine_channel  chan int // machine_channel can be used to pass Stop-Signal to stop the machine

	RaftInbox  chan interface{}
	RaftOutbox chan interface{}

	Log          []cluster.LogEntry
	LastLogIndex int64
	CommitIndex  int64
	LastApplied  int64
	NextIndex    map[int]int64
	MatchIndex   map[int]int64
	FollowerTerm map[int]int
}

/******************************************************************************************
	Main() method , starts the proceedings by sending the Server in Follower phase
******************************************************************************************/
func main() {

	gob.Register(cluster.AppendEntriesRequestStruct{})
	gob.Register(cluster.AppendEntriesResponseStruct{})
	gob.Register(cluster.RequestVoteRequestStruct{})
	gob.Register(cluster.RequestVoteResponseStruct{})

	myServerId, _ := strconv.Atoi(os.Args[1])
	//serverStateFile := os.Args[2]
	//configFile := os.Args[3]
	//minTimeOut, _ := strconv.Atoi(os.Args[4])
	//maxTimeOut, _ := strconv.Atoi(os.Args[5])

	serverStateFile := "./serverState.json"
	configFile := "./config.json"
	minTimeOut := 5000
	maxTimeOut := 6000

	fmt.Println("Starting Main", myServerId)

	machine := NewMachine(myServerId, serverStateFile, configFile, minTimeOut, maxTimeOut)

	machine.Log = make([]cluster.LogEntry, 0)
	machine.LastLogIndex = -1
	machine.CommitIndex = -1
	machine.LastApplied = -1
	machine.NextIndex = make(map[int]int64)
	machine.MatchIndex = make(map[int]int64)
	machine.FollowerTerm = make(map[int]int)

	for _, Sid := range machine.server.Peers() {

		machine.NextIndex[Sid] = int64(0)
		machine.MatchIndex[Sid] = int64(-1)
		machine.FollowerTerm[Sid] = 0
	}

	//***********For Tester Purpose Only***********
	/*testerPort := os.Args[6]

	var t Testerstruct

	t.TesterInit(&machine, testerPort)
	go t.TesterRecv(&machine)
	*/
	//*********Tester Methods Closed***************
	go cluster.ReceiveMessage(&machine.server)

	go cluster.SendMessage(&machine.server)

	go machine.StartRaftInbox()

	fmt.Println("\nElection TimeOut", machine.Election_Timeout())

	Start(&machine)

}
