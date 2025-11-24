package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
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
	BidderHighestBids    map[string]int32 // bidder_id -> highest bid
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

	go server.connectToPeer()

	if !server.isPrimary {
		go server.monitorPrimary()
	}

	//start grpc server

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", server.port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterAuctionServiceServer(grpcServer, server)

	log.Printf("Server %d listening on port %d", server.ServerID, server.port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}

func initializeAuctionState(durationSeconds int64) *AuctionState {
	now := time.Now().UnixMilli()
	return &AuctionState{
		BidderHighestBids:    make(map[string]int32),
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isPrimary {
		return &pb.BidResponse{
			Outcome: pb.BidResponse_EXCEPTION,
			Message: "This server is not the primary. Please contact the primary server.",
		}, nil
	}

	bidderID := req.BidderId
	amount := req.Amount

	now := time.Now().UnixMilli()
	if now >= s.state.AuctionEndTime {
		return &pb.BidResponse{
			Outcome: pb.BidResponse_FAIL,
			Message: "Auction has ended.",
		}, nil
	}

	if bidderID == "" {
		return &pb.BidResponse{
			Outcome: pb.BidResponse_FAIL,
			Message: "Bidder ID cannot be empty.",
		}, nil
	}

	previousBid, exists := s.state.BidderHighestBids[bidderID]

	if !exists {
		s.state.RegisteredBidders = append(s.state.RegisteredBidders, bidderID)
		log.Printf("Registered new bidder: %s", bidderID)
	} else {
		if amount <= previousBid {
			return &pb.BidResponse{
				Outcome: pb.BidResponse_FAIL,
				Message: fmt.Sprintf("Bid must be hight than the pevious bid (%d)", previousBid),
			}, nil
		}
	}

	if amount <= s.state.CurrentHighestBid {
		return &pb.BidResponse{
			Outcome: pb.BidResponse_FAIL,
			Message: fmt.Sprintf("Bid must be hight than the current highest bid (%d)", s.state.CurrentHighestBid),
		}, nil
	}

	s.state.BidderHighestBids[bidderID] = amount
	s.state.CurrentHighestBidder = bidderID
	s.state.CurrentHighestBid = amount

	log.Printf("Bid accepted! %s is now winning with $%d", bidderID, amount)

	// REPLICATION TO BACKUP HERE
	if s.peerClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		stateUpdate := &pb.StateUpdate{
			SenderServerId: s.ServerID,
			State:          s.convertStateToProto(),
			Timestamp:      time.Now().UnixMilli(),
		}

		ack, err := s.peerClient.ReplicateState(ctx, stateUpdate)
		if err != nil {
			log.Printf("Warning: Failed to replicate state to backup server: %v", err)
		} else if ack.Success {
			log.Printf("Success: State replicated sucessfully to backupserver %d", ack.ServerId)
		}
	}

	return &pb.BidResponse{
		Outcome: pb.BidResponse_SUCCESS,
		Message: fmt.Sprintf("Your bid was accepted! You are currently winning with: $%d", amount),
	}, nil
}

func (s *Server) Result(ctx context.Context, req *pb.ResultRequest) (*pb.ResultResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	auctionEnded := now >= s.state.AuctionEndTime || s.state.AuctionEnded

	var timeRemaining int64
	if !auctionEnded {
		timeRemaining = (s.state.AuctionEndTime - now) / 1000 //conv to sec
	}

	response := &pb.ResultResponse{
		AuctionEnded:  auctionEnded,
		HighestBidder: s.state.CurrentHighestBidder,
		HighestBid:    s.state.CurrentHighestBid,
		TimeRemaining: timeRemaining,
	}

	if auctionEnded {
		log.Printf("Result query - Auction ended. Winner is: %s with $%d. WOW!", s.state.CurrentHighestBidder, s.state.CurrentHighestBid)
	} else {
		log.Printf("Result query - Current leader is: %s with $%d. Time remaining: %d seconds", s.state.CurrentHighestBidder, s.state.CurrentHighestBid, timeRemaining)
	}

	return response, nil
}

func (s *Server) ReplicateState(ctx context.Context, req *pb.StateUpdate) (*pb.StateAck, error) {
	log.Printf("State replication recieved from server %d ", req.SenderServerId)

	pbState := req.State

	s.state.BidderHighestBids = pbState.BidderHighestBids
	s.state.CurrentHighestBidder = pbState.CurrentHighestBidder
	s.state.CurrentHighestBid = pbState.CurrentHighestBid
	s.state.AuctionStartTime = pbState.AuctionStartTime
	s.state.AuctionEndTime = pbState.AuctionEndTime
	s.state.AuctionEnded = pbState.AuctionEnded
	s.state.RegisteredBidders = pbState.RegisteredBidders

	log.Printf("Replicated state from server %d", req.SenderServerId)

	return &pb.StateAck{
		Success:      true,
		ServerId:     s.ServerID,
		ErrorMessage: "",
	}, nil

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

func (s *Server) convertStateToProto() *pb.AuctionState {
	return &pb.AuctionState{
		BidderHighestBids:    s.state.BidderHighestBids,
		CurrentHighestBidder: s.state.CurrentHighestBidder,
		CurrentHighestBid:    s.state.CurrentHighestBid,
		AuctionStartTime:     s.state.AuctionStartTime,
		AuctionEndTime:       s.state.AuctionEndTime,
		AuctionEnded:         s.state.AuctionEnded,
		RegisteredBidders:    s.state.RegisteredBidders,
	}
}
