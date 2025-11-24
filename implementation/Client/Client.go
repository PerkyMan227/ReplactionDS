package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	pb "github.com/PerkyMan227/ReplactionDS/proto/proto"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn   *grpc.ClientConn
	client pb.AuctionServiceClient
	id     string
}

type Outcome int

const (
	Success Outcome = iota
	Fail
	Exception
)

type Ack struct {
	Outcome Outcome
}

func main() {
	//define flag
	idFlag := flag.String("id", "", "Unique ID for the bidder")
	flag.Parse()

	// If not provided generate a random UUID
	bidderID := *idFlag
	if bidderID == "" {
		bidderID = uuid.New().String()
	}

	fmt.Println("Using bidder ID:", bidderID)

	//connect to server
	connection, err := grpc.NewClient("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer connection.Close()

	client := pb.NewAuctionServiceClient(connection)
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "exit" {
			fmt.Println("Exiting client.")
			return
		}

		// Handle "result"
		if input == "result" {
			handleResult(client)
			continue
		}

		// Handle "bid <amount>"
		var amount int32
		n, _ := fmt.Sscanf(input, "bid %d", &amount)
		if n == 1 {
			handleBid(client, bidderID, amount)
			continue
		}

		fmt.Printf("Unknown command. Use: bid <amount>, result, exit... Recieved input: %s", input)
		fmt.Println()
		// WHAT DO??
		// if reader.equals("bid")

		// BID -> send bid request to server with res, err := client.Bid(id, amount)

		// GET RESULT -> send request to server for result with client.Result()
	}

}

func handleBid(client pb.AuctionServiceClient, bidderID string, amount int32) {

	withFailover(func(c pb.AuctionServiceClient) error {

		req := &pb.BidRequest{BidderId: bidderID, Amount: amount}
		res, err := c.Bid(context.Background(), req)
		if err != nil {
			fmt.Println("Error sending bid:", err)
			return err
		}
		fmt.Printf("Outcome: %s | %s\n", res.Outcome.String(), res.Message)
		return nil
	})
}

func handleResult(client pb.AuctionServiceClient) {

	withFailover(func(c pb.AuctionServiceClient) error {

		req := &pb.ResultRequest{}
		res, err := c.Result(context.Background(), req)
		if err != nil {
			fmt.Println("Error getting result:", err)
			return err
		}
		fmt.Println("Auction Over:", res.AuctionEnded)
		fmt.Println("Highest Bid:", res.HighestBid)
		fmt.Println("Winner:", res.HighestBidder)
		return nil
	})
}

// helper function that tries to connect to port 5001, and if that fails, tries to connect to port 5002

var useBackup bool = false

func withFailover(call func(pb.AuctionServiceClient) error) {
	// Try port 5001
	if useBackup {
		conn2, err := grpc.NewClient("localhost:5002", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Println("Backup server failed:", err)
			return
		}
		defer conn2.Close()

		client2 := pb.NewAuctionServiceClient(conn2)
		call(client2)
		return
	}

	//try primary (5001) first time
	conn1, err := grpc.NewClient("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		client1 := pb.NewAuctionServiceClient(conn1)

		if call(client1) == nil {
			conn1.Close()
			return
		}
		conn1.Close()
	}

	// Primary failed =>switch permanently
	fmt.Println("Primary failed — switching permanently to backup.")
	useBackup = true

	//Use backup
	conn2, err := grpc.NewClient("localhost:5002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("Backup server also failed:", err)
		return
	}
	defer conn2.Close()

	client2 := pb.NewAuctionServiceClient(conn2)
	call(client2)
}
