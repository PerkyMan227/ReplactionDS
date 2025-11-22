package main

import (
	"fmt"
	"log"
	pb "github.com/PerkyMan227/ReplactionDS/proto/proto"
	"flag"
	"bufio"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"github.com/google/uuid"
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

type Result struct {
	AuctionOver bool   // true if auction ended
	HighestBid  int    // current highest bid
	Winner      string // bidder ID of winner (if auction ended)
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
		// WHAT DO??
		// if reader.equals("bid")

		// BID -> send bid request to server with res, err := client.Bid(id, amount)

		// GET RESULT -> send request to server for result with client.Result()
	}







}

func handleBid(client pb.AuctionServiceClient, bidderID string, amount int64) {
    req := &pb.BidRequest{
        BidderId: bidderID,
        Amount:   amount,
    }

    res, err := client.Bid(context.Background(), req)
    if err != nil {
        fmt.Println("Error sending bid:", err)
        return
    }

    fmt.Printf("Outcome: %s | %s\n", res.Outcome.String(), res.Message)
}

func handleResult(client pb.AuctionServiceClient) {
    req := &pb.ResultRequest{}

    res, err := client.Result(context.Background(), req)
    if err != nil {
        fmt.Println("Error getting result:", err)
        return
    }

    fmt.Println("Auction Over:", res.AuctionOver)
    fmt.Println("Highest Bid:", res.HighestBid)
    fmt.Println("Winner:", res.Winner)
}

