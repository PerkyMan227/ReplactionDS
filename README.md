## Auction System: Quick Start Guide

This guide provides the minimal steps necessary to run the two server nodes and the client using the go run command.

1. Run the Servers

You need two servers running simultaneously. Open two separate terminal windows.
Server & Role
Node 1 (Primary): `go run server/Server.go -id=1 -port=5001 -peer=localhost:5002 -duration=300`	
Node 2 (Backup):  `go run server/Server.go -id=2 -port=5002 -peer=localhost:5001 -duration=300`	


2. Run the Client

Open a third terminal window to run the client. Provide a unique number (ID) for your bidder.
Bash. Chose any non-duplicate number as ID

`go run Client.go -id=<ID>`

3. Client Commands

Use the following commands in the client terminal:
Command	Action	Example
`bid <amount>`	Place a bid. Must be higher than the current highest bid.
`result`	Check the auction's status (winner, highest bid, time left).	
`exit`	Close the client.	
