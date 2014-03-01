Raft
=====

cluster.go
----------

cluster.go is the source file which is used to generate cluster.a library file.
There are many methods defined in this library which internally implements [zmq].

Few methods are described below :- 

>  - NewServer(serverId int, configFile string) Server
>   * It will return a Server object by passing serverId as one of the argument , other details of the Server are stored in configFile . These details can be IP address (alongwith port number)
>  - SendMessage(server Server)
>    * It will start sending any message (in the format as mentioned in cluster.Envelope) as soon as invoked.
>  - ReceiveMessage(server Server)
>    * It will start receiving on the mentioned port (in the format as mentioned in cluster.Envelope) as soon as invoked. 


Installation
--------------

```sh
go get github.com/RaviKumarYadav/Raft/cluster
cd .../github.com/RaviKumarYadav/Raft/cluster
go test

```

How it works
-------------
```sh
Check the configFile as it should be present with same folder of the cluster_test.go file. 

```
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
