// ===================================================================================
// TOLEREX – MEMBER SERVER (DATA PLANE / STORAGE WORKER)
// ===================================================================================
//
// Member node responsibilities:
// - Accept Store/Retrieve exclusively from Leader over mTLS
// - Persist messages to local disk via storage package
// - No coordination/replication logic
//
// Improvements in this version:
// (1) Strong caller authorization using TLS VerifiedChains + expected Leader CN
// (3) Context cancellation checks around disk I/O (timeout-aware behavior)
//
// ===================================================================================

package server

import (
	"context"
	"time"

	"tolerex/internal/logger"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Expected Leader identity (certificate Common Name).
// Ensure leader certificate CN is exactly "leader".
const expectedLeaderCN = "leader"

// ===================================================================================
// CALLER IDENTITY EXTRACTION
// ===================================================================================

// callerFromContext extracts the caller identity from the gRPC context using mTLS peer info.
// Returns CN from peer certificate if available, otherwise "unknown".
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

// authorizeLeader enforces that only the genuine Leader (verified by TLS chain + CN)
// can call Store/Retrieve.
// This is defense-in-depth on top of mTLS.
func authorizeLeader(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing peer info")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing TLS auth info")
	}

	// VerifiedChains means TLS verified certificate chain against server's trusted CA.
	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return status.Error(codes.Unauthenticated, "could not verify peer certificate")
	}

	leaf := tlsInfo.State.VerifiedChains[0][0]
	if leaf.Subject.CommonName != expectedLeaderCN {
		return status.Error(codes.PermissionDenied, "unauthorized caller")
	}

	return nil
}

// ctxCancelled is a small helper for cooperative cancellation.
func ctxCancelled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		// Preserve gRPC semantics: canceled or deadline exceeded.
		if ctx.Err() == context.DeadlineExceeded {
			return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
		}
		return status.Error(codes.Canceled, "request cancelled")
	default:
		return nil
	}
}

// ===================================================================================
// MEMBER SERVER DEFINITION
// ===================================================================================

type MemberServer struct {
	pb.UnimplementedStorageServiceServer
	DataDir string
	IOMode  string // buffered | unbuffered
}

// ===================================================================================
// gRPC: STORE (WRITE PATH)
// ===================================================================================

func (s *MemberServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {

	if err := authorizeLeader(ctx); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, "request cancelled")
	default:
	}

	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, "cancelled before write")
	default:
	}

	err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text, s.IOMode)
	if err != nil {
		return &pb.StoreResult{Ok: false, Err: err.Error()}, nil
	}

	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, "cancelled after write")
	default:
	}

	logger.Info(log, "store ok msg_id=%d duration=%s", msg.Id, time.Since(start))
	return &pb.StoreResult{Ok: true}, nil
}

// ===================================================================================
// gRPC: RETRIEVE (READ PATH)
// ===================================================================================

func (s *MemberServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	// (1) Authorize caller
	if err := authorizeLeader(ctx); err != nil {
		logger.Warn(logger.Member, "unauthorized Retrieve attempt")
		return nil, err
	}

	// (3) Cancellation-aware
	if err := ctxCancelled(ctx); err != nil {
		return nil, err
	}

	start := time.Now()
	log := logger.WithContext(ctx, logger.Member)

	caller := callerFromContext(ctx)

	logger.Info(
		log,
		"retrieve begin caller=%s msg_id=%d",
		caller,
		req.Id,
	)

	// (3) Cancellation-aware before disk I/O
	if err := ctxCancelled(ctx); err != nil {
		logger.Warn(log, "retrieve cancelled before disk read msg_id=%d", req.Id)
		return nil, err
	}

	text, err := storage.ReadMessage(s.DataDir, int(req.Id))
	if err != nil {
		logger.Info(
			log,
			"retrieve miss caller=%s msg_id=%d duration=%s",
			caller,
			req.Id,
			time.Since(start),
		)
		return &pb.StoredMessage{}, nil
	}

	// (3) Cancellation-aware after disk I/O
	if err := ctxCancelled(ctx); err != nil {
		logger.Warn(log, "retrieve cancelled after disk read msg_id=%d", req.Id)
		return nil, err
	}

	logger.Info(
		log,
		"retrieve hit caller=%s msg_id=%d duration=%s",
		caller,
		req.Id,
		time.Since(start),
	)

	return &pb.StoredMessage{
		Id:   req.Id,
		Text: text,
	}, nil
}
