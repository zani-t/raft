# Raft Algorithm

This is an implementation of the Raft Algorithm developed by Diego Ongaro and John Ousterhout. I wrote this under the guidance of Eli Bendersky's tutorial and the original paper by Ongaro and Ousterhout:

https://eli.thegreenplace.net/2020/implementing-raft-part-0-introduction/

https://raft.github.io/raft.pdf

**What is Raft?**
Raft, as a consensus algorithm, has the purpose of maintaining reliability of a distributed computing system in the event of one or multiple failures. As an example, a value modified in a database by a far-away client needs to be recorded accurately even if certain servers are temporarily down or have faulty connection. The Raft algorithm was developed with the paramount goal of being understandable, specifically more so than the previous industry standard of the Paxos algorithm.

Raft works by enforcing some core concepts and separating the ideas of a consensus problem into distinct steps.
- First step - Leader election: Leaders are elected if they have waited too long for a response from the previous leader, and if a majority of nodes in the system give the leader a vote. From this time all changes are processed and "double checked" by the leader.
- Second step - Log replication: On reception of an instruction from a client, the leader adds an instruction to a log. Confirmation of the instruction happens when a majority of nodes tell the leader that they have received the instruction.

This was written as my way of learning about distributed computing and consensus algorithms, which are known to be inherently complex. The Raft paper itself provides a through explanation of the algorithm, it goals, and edge cases. However for a simpler and more introductory explanation, this visualization greatly helped me:
http://thesecretlivesofdata.com/raft/

**Implementation**
Raft is normally incorporated into a distributed computing services such as a group of key-value store--in contrast the code in this repository contains enough to send a series of instructions to a simulated cluster of servers, create logs of instructions, and change server states to test different scenarios. Environment variables can also be set to perform some simulated network inefficiencies.
Run ./test.sh to test. Source code files include:
- raft.go: Algorithm specific operations
- server.go: Simulated server implementation
- storage.go: Simulation of persistent storage (of server/consensus module states)
- Testing implementation written by Bendersky
