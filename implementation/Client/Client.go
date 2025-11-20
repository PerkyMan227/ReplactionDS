package Client

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
		fmt.Print("Enter your bid amount (or type 'result' to get the auction result): ")
		input, _ := reader.ReadString('\n')
		input = input[:len(input)-1] // remove newline character
		if input == "result" {
			result, err := getResult(client)
			if err != nil {
				log.Printf("Error getting result: %v", err)
				continue
			}
			if result.AuctionOver {
				fmt.Printf("Auction over! Winner: %s with bid: %d\n", result.Winner, result.HighestBid)
				break
			} else {
				fmt.Printf("Current highest bid: %d\n", result.HighestBid)
			}
		} else {
			var amount int
			_, err := fmt.Sscanf(input, "%d", &amount)
			if err != nil {
				fmt.Println("Invalid input. Please enter a valid bid amount.")
				continue
			}
			ack, err := placeBid(client, bidderID, amount)
			if err != nil {
				log.Printf("Error placing bid: %v", err)
				continue
			}
			switch ack.Outcome {
			case Success:
				fmt.Println("Bid accepted!")
			case Fail:
				fmt.Println("Bid too low, try again.")
			case Exception:
				fmt.Println("An error occurred while placing your bid.")
			}
		}
	}
}

func bid(bidderID string, amount int) (Ack, error) {
	//given a bid, returns an outcome among {fail, success or exception}
	//brug servers bid() metode

}

func (s *Server) result() (Result, error) {
	//if the auction is over, it returns the result, else highest bid.
	//brug servers result() metode
}
