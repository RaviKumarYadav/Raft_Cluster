RAFT
=========

Raft layer simply means Distributed Consensus i.e. Agreement of various servers/nodes over something which in our case is 'data' , it can be as simple as log entries.

RAFT has following parts :-
  - Leader Election
  - Log Replication

Assignment
----------

> I have implemented 'Leader Election' part under this assignment , which means how various nodes come together and elect one of the node as leader. This node then will obtain request from client (outer world).


It elect a Leader and if any Leader goes unfunctional or get disconnected then our program will elect a new Leader from remaining alive nodes (Note :- A leader must have more than half of the total votes).


Roles of Server
---------------

```
- Follower
- Candidate
- Leader
```


A Server/Node can act as :-
- Follower
 * All Server starts as Follower. 
- Candidate
 * A Follower becomes Candidate if it waits more than that of its election timeout without receiving any 'Append_Entries' or 'Request_Vote' request.
- Leader
 * A Candidate when gets more than half of the votes (of the total server in cluster) becomes a Leader.


Installation
--------------

```sh
go get https://github.com/RaviKumarYadav/Raft_Cluster.git
cd ../github.com/RaviKumarYadav/Raft_Cluster
go test
```

License
----

[IIT Bombay]

[IIT Bombay]:http://www.cse.iitb.ac.in/

    
