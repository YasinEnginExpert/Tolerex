// ===================================================================================
// TOLEREX – MEMBER SERVER (DATA PLANE / STORAGE WORKER)
// ===================================================================================
//
// This file implements the Member-side gRPC service responsible for
// persistent storage operations in the Tolerex distributed system.
//
// From a distributed systems perspective, a Member node:
//
// - Acts as a stateful storage replica
// - Accepts Store / Retrieve requests exclusively from the Leader
// - Persists data to local disk
// - Does not perform replication or coordination logic
//
// Key responsibilities implemented here:
//
// - Verifying the caller identity using mTLS peer information
// - Writing messages to disk in an isolated data directory
// - Reading messages from disk upon request
// - Logging request lifecycle events with latency measurements
//
// Security model:
//
// - Caller identity is extracted from the mTLS certificate (Common Name)
// - Only mutually authenticated gRPC connections are accepted
//
// This file contains no cluster-level logic.
// All orchestration is handled by the Leader.
//
// ===================================================================================

package server

import (
	"context"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// ===================================================================================
// CALLER IDENTITY EXTRACTION
// ===================================================================================

// --- CALLER FROM CONTEXT ---
// Extracts the caller identity from the gRPC context using mTLS peer info.
//
// Returns:
// - CommonName (CN) from the peer certificate if available
// - "unknown" if identity cannot be determined
func callerFromContext(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "unknown"
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return "unknown"
	}

	return tlsInfo.State.PeerCertificates[0].Subject.CommonName
}

// ===================================================================================
// MEMBER SERVER DEFINITION
// ===================================================================================

// --- MEMBER SERVER ---
// Implements the generated StorageService gRPC interface
// and handles local disk persistence.
type MemberServer struct {
	pb.UnimplementedStorageServiceServer
	DataDir string
}

// ===================================================================================
// gRPC: STORE (WRITE PATH)
// ===================================================================================

// --- STORE ---
// Persists the incoming message to disk.
//
// Expected caller:
// - Leader node (validated via mTLS)
//
// Behavior:
// - Writes message content to a file under DataDir
// - Logs success or failure along with execution latency
func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	// --- CALLER IDENTIFICATION ---
	caller := callerFromContext(ctx)

	logger.Info(
		log,
		"Store start | caller=%s | msg_id=%d",
		caller,
		msg.Id,
	)

	// --- DISK WRITE ---
	err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text)
	if err != nil {
		logger.Error(
			log,
			"Store failed | caller=%s | msg_id=%d | err=%v",
			caller,
			msg.Id,
			err,
		)
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil
	}

	logger.Info(
		log,
		"Store success | caller=%s | msg_id=%d | duration=%s",
		caller,
		msg.Id,
		time.Since(start),
	)

	return &pb.StoreResult{Ok: true}, nil
}

// ===================================================================================
// gRPC: RETRIEVE (READ PATH)
// ===================================================================================

// --- RETRIEVE ---
// Reads the requested message from disk.
//
// Behavior:
// - Attempts to load message content from DataDir
// - Returns empty response if the message does not exist
// - Logs hit/miss along with execution latency
func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	// --- CALLER IDENTIFICATION ---
	caller := callerFromContext(ctx)

	logger.Info(
		log,
		"Retrieve start | caller=%s | msg_id=%d",
		caller,
		req.Id,
	)

	// --- DISK READ ---
	text, err := storage.ReadMessage(s.DataDir, int(req.Id))
	if err != nil {
		logger.Warn(
			log,
			"Retrieve miss | caller=%s | msg_id=%d | err=%v",
			caller,
			req.Id,
			err,
		)
		return &pb.StoredMessage{}, nil
	}

	logger.Info(
		log,
		"Retrieve success | caller=%s | msg_id=%d | duration=%s",
		caller,
		req.Id,
		time.Since(start),
	)

	return &pb.StoredMessage{
		Id:   req.Id,
		Text: text,
	}, nil
}
