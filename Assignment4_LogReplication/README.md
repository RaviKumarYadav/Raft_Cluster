RAFT - Log Replication
=======

cluster.go
----------

cluster.go is the source file which is used to generate cluster.a library file.
There are many methods defined in this library which internally implements [zmq].

raft.go
-------

raft.go implements the Server logic and how it should behave if its in different phases .
 - Roles of Server
 
```
- Follower
- Candidate
- Leader
```

RAFT has following parts :-
  - Leader Election
    - A Server is elected as a Leader if it has the majority i.e. Votes greater than half of the total number of servers in cluster.
 
  - Log Replication
     - This part is responsible for the consistency of data i.e. Data at Leader must also be reflected or simply stored on atleast half of the total servers in cluster.
 

Installation
--------------

```sh
go get github.com/RaviKumarYadav/Raft_Cluster/Assignment4_LogReplication
cd .../github.com/RaviKumarYadav/Raft_Cluster/Assignment4_LogReplication
go test

```

How it works
-------------

 > - Cluster is a group of different Servers working and communicating with each other to form a larger system that will later be used for implementing distributed system.

 > - Server uses Messages to communicate with each other . Each server is assigned a unique ServerId which helps in identifying any Server in the System/Cluster.

 > - Message being exchanged also have a structure which contains :-
 * MessageId
    -   Its unique for each Message.
 * Pid
    -   It reflects the target server or the recipient server for the message.
 * MsgType
    -   Messages can be of different types like :- 
        - AppendEntriesRequest
        - AppendEntriesResponse
        - RequestVoteRequest
	    - RequestVoteResponse
 * SenderId
    - ServerId of the sending server.
 * Term
    - Term of the Server sending the message.
 * Voted
    - Voting Status of the Server for the said term.


To run individually just pass "ServerId" as command line argument eg :- go run mainFile.go 1 , where "1" is the ServerId whose other details are present in configFile which will be fetched by the program.


Test Cases
-----------

There are few cases present in file "Raft/cluster/cluster_test.go". It tests for following :-

* Unicast
    * Peer-Peer messages sent by one server to another (by each server in cluster)
* Multicast
    * Messages sent by each server is listened by each member(server) in the  cluster
* Unicast and Multicast
    * Mix of Unicast and Multicast messages were sent by each server
* Cyclic Unicast
    * Each server sent messages to another server which next to it (in terms of ServerId)


How to Run Test Cases
-----------------------

```sh
cd github.com/RaviKumarYadav/Raft/cluster
go test
```



License
----

[IIT Bombay]

[zmq]:http://zeromq.org/
[IIT Bombay]:http://www.cse.iitb.ac.in/
