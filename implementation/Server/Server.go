package main

import (
	"context"
	"flag"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/PerkyMan227/ReplactionDS/proto/proto"
)

type Server struct {
	pb.UnimplementedAuctionServiceServer

	ServerID int32
	port     int
	peerAddr string

	isPrimary bool
	mu        sync.RWMutex

	state      *AuctionState
	peerConn   *grpc.ClientConn
	peerClient pb.AuctionServiceClient
}

type AuctionState struct {
	BidderHighestBid     map[string]int32 // bidder_id -> highest bid
	CurrentHighestBidder string
	CurrentHighestBid    int32
	AuctionStartTime     int64 //milisec
	AuctionEndTime       int64 //milisec
	AuctionEnded         bool
	RegisteredBidders    []string
}

func main() {
	ServerID := flag.Int("id", 1, "ServerID (1 or 2)")
	port := flag.Int("port", 5001, "Port to listen on")
	peerAddr := flag.String("peer", "localhost:5002", "Address for peer server")
	auctionDuration := flag.Int("duration", 100, "Auction duration in seconds")
	flag.Parse()

	server := &Server{
		ServerID: int32(*ServerID),
		port:     *port,
		peerAddr: *peerAddr,
		state:    initializeAuctionState(int64(*auctionDuration)),
	}

	server.isPrimary = (server.ServerID == 1)

	if server.isPrimary {
		log.Printf("Server %d is starting as PRIMARY on port %d", server.ServerID, server.port)
	} else {
		log.Printf("Server %d is starting as BACKUP on port %d", server.ServerID, server.port)
	}
}

func initializeAuctionState(durationSeconds int64) *AuctionState {
	now := time.Now().UnixMilli()
	return &AuctionState{
		BidderHighestBid:     make(map[string]int32),
		CurrentHighestBidder: "",
		CurrentHighestBid:    0,
		AuctionStartTime:     now,
		AuctionEndTime:       now + (durationSeconds * 1000),
		AuctionEnded:         false,
		RegisteredBidders:    []string{},
	}
}

func (s *Server) connectToPeer() {
	for {
		conn, err := grpc.NewClient(s.peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect to peer at %s: %v. Retrying...", s.peerAddr, err)
			time.Sleep(3 * time.Second)
			continue
		}

		s.peerConn = conn
		s.peerClient = pb.NewAuctionServiceClient(conn)
		log.Printf("Connected to peer server at %s", s.peerAddr)
		return
	}
}
func (s *Server) monitorPrimary() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	missedHeartbeats := 0
	maxMissed := 3

	for range ticker.C {
		s.mu.RLock()
		if s.isPrimary {
			s.mu.RUnlock()
			continue
		}
		s.mu.RUnlock()

		if s.peerClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			_, err := s.peerClient.Heartbeat(ctx, &pb.HeartbeatRequest{
				ServerId:  s.ServerID,
				Timestamp: time.Now().UnixMilli(),
			})
			cancel()

			if err != nil {
				missedHeartbeats++
				log.Printf("Heartbeat missed (%d/%d): %v", missedHeartbeats, maxMissed, err)

				if missedHeartbeats >= maxMissed {
					log.Printf("Primary appears to be down. Starting election...")
					s.startElection()
					missedHeartbeats = 0
				}
			} else {
				missedHeartbeats = 0
			}
		}
	}
}

func (s *Server) startElection() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Server %d becoming PRIMARY", s.ServerID)
	s.isPrimary = true
}

func (s *Server) Bid(ctx context.Context, req *pb.BidRequest) (*pb.BidResponse, error) {
	return &pb.BidResponse{
		Outcome: pb.BidResponse_EXCEPTION,
		Message: "Not implemented yet",
	}, nil
}

func (s *Server) Result(ctx context.Context, req *pb.ResultRequest) (*pb.ResultResponse, error) {
	return &pb.ResultResponse{}, nil
}

func (s *Server) ReplicateState(ctx context.Context, req *pb.StateUpdate) (*pb.StateAck, error) {
	return &pb.StateAck{Success: true, ServerId: s.ServerID}, nil
}

func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &pb.HeartbeatResponse{
		Alive:     true,
		IsPrimary: s.isPrimary,
		ServerId:  s.ServerID,
	}, nil
}

func (s *Server) StartElection(ctx context.Context, req *pb.ElectionRequest) (*pb.ElectionResponse, error) {
	return &pb.ElectionResponse{Ok: false, ResponderServerId: s.ServerID}, nil
}

func (s *Server) AnnounceLeader(ctx context.Context, req *pb.LeaderAnnouncement) (*pb.LeaderAck, error) {
	return &pb.LeaderAck{Acknowledged: true, ServerId: s.ServerID}, nil
}	