Bug
===

cluster_test.go contains a bug , I have outlined the region with string "Error Prone Code" , so just search for it. This region contains both correct code and erroneous code , just comment one and uncomment other to see the behaviour.

Code Region looks like  :- 

```sh
        // *********** Error Prone Code ********************	
		// 1. Corrected code , which calls a function created below
		// Uncomment following line to work smoothly without exception (and comment option 2)
		// go receive_always(serverArray[tempServerId], &received_count)
		
		// 2. Errorneous code , which create a go-routine here itself and somehow throws exception
		// Comment following code to work smoothly and Uncomment option 1
		go func() {
			fmt.Println("Value of i :- " , tempServerId)
			_ = <-serverArray[tempServerId].Inbox()
			mutex1.Lock()
			received_count += 1
			mutex1.Unlock()
		}()
		
		// ********** Ends *********************************

```

Steps to Check/Debug
---------------------

```sh
go get github.com/RaviKumarYadav/Raft/Bug
cd .../github.com/RaviKumarYadav/Raft/Bug
go test

```

How it works
-------------
```sh
 1. Check the configFile as it should be present with same folder of the cluster_test.go file.
 
 2. There are two set of code marked by "1." and "2." in above snapshot. Please comment one part and Uncomment another to see the effect.
 
 3. Second test-case will fail as issue with consecutive test-case execution has not been resolved yet. So just check result of first test-case only.
```

To run individually just pass "ServerId" as command line argument eg :- go run mainFile.go 1 , where "1" is the ServerId whose other details are present in configFile which will be fetched by the program.

License
----

[IITB]

[zmq]:http://zeromq.org/
[IITB]:http://www.cse.iitb.ac.in/
