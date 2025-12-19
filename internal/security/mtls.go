// ===================================================================================
// TOLEREX – MUTUAL TLS (mTLS) SECURITY CONFIGURATION
// ===================================================================================
//
// This file defines the mutual TLS (mTLS) configuration utilities used to secure
// all gRPC communication within the Tolerex distributed system.
//
// From a security and systems perspective, this package:
//
// - Establishes strong, certificate-based authentication for both peers
// - Eliminates reliance on network-level trust (e.g., IP-based security)
// - Enforces identity verification using a private Certificate Authority (CA)
// - Protects against man-in-the-middle (MITM) attacks
// - Provides cryptographic isolation between Leader and Member nodes
//
// Two distinct trust roles are implemented:
//
// 1) Server-side mTLS credentials
//    - Presents server certificate
//    - Requires and validates client certificates
//
// 2) Client-side mTLS credentials
//    - Presents client certificate
//    - Verifies server identity via CA and ServerName
//
// TLS 1.3 is enforced to ensure modern cryptographic guarantees.
//
// ===================================================================================

package security

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"google.golang.org/grpc/credentials"
)

// --- SERVER-SIDE mTLS CREDENTIALS ---
// Creates TransportCredentials for a gRPC server that:
//
// - Loads its own certificate and private key
// - Requires clients to present valid certificates
// - Verifies client certificates against a trusted CA
// - Enforces TLS 1.3
func NewMTLSServerCreds(
	certFile, keyFile, caFile string,
) (credentials.TransportCredentials, error) {

	// --- SERVER CERTIFICATE LOADING ---
	// Loads the server's X.509 certificate and private key.
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	// --- CA CERTIFICATE LOADING ---
	// Reads the trusted Certificate Authority certificate.
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	// --- CA CERT POOL CREATION ---
	// Builds a certificate pool used to validate client certificates.
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	// --- TLS CONFIGURATION ---
	// Configures the server to:
	// - Present its certificate
	// - Require and verify client certificates
	// - Reject TLS versions below 1.3
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}

// --- CLIENT-SIDE mTLS CREDENTIALS ---
// Creates TransportCredentials for a gRPC client that:
//
// - Presents its own certificate to the server
// - Verifies the server certificate using a trusted CA
// - Validates the server identity via ServerName (SAN / CN)
// - Enforces TLS 1.3
func NewMTLSClientCreds(
	certFile, keyFile, caFile, serverName string,
) (credentials.TransportCredentials, error) {

	// --- CLIENT CERTIFICATE LOADING ---
	// Loads the client's X.509 certificate and private key.
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	// --- CA CERTIFICATE LOADING ---
	// Reads the trusted Certificate Authority certificate.
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	// --- CA CERT POOL CREATION ---
	// Builds a certificate pool used to validate the server certificate.
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	// --- TLS CONFIGURATION ---
	// Configures the client to:
	// - Present its certificate
	// - Verify the server certificate chain
	// - Validate the server hostname
	// - Reject TLS versions below 1.3
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}
