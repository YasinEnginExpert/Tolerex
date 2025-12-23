// ===================================================================================
// TOLEREX – MUTUAL TLS (mTLS) SECURITY CONFIGURATION
// ===================================================================================
//
// This package defines the mutual TLS (mTLS) configuration utilities used to
// secure all gRPC communication within the Tolerex distributed system.
//
// From a security and systems-design perspective, this package:
//
//   - Establishes strong, certificate-based authentication for both peers
//   - Eliminates reliance on network-level trust (e.g. IP-based security)
//   - Enforces identity verification using a private Certificate Authority (CA)
//   - Protects against man-in-the-middle (MITM) attacks
//   - Provides cryptographic isolation between Leader and Member nodes
//
// Two distinct trust roles are implemented:
//
//   1) Server-side mTLS credentials
//      - Presents a server certificate
//      - Requires and verifies client certificates
//
//   2) Client-side mTLS credentials
//      - Presents a client certificate
//      - Verifies the server identity using CA trust and ServerName validation
//
// Security guarantees:
//
//   - TLS 1.3 is enforced (modern cipher suites only)
//   - Both peers must possess valid certificates
//   - All communication is encrypted and authenticated
//
// ===================================================================================

package security

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"google.golang.org/grpc/credentials"
)

// ===================================================================================
// SERVER-SIDE mTLS CREDENTIALS
// ===================================================================================
//
// NewMTLSServerCreds creates TransportCredentials for a gRPC server that:
//
//   - Loads its own X.509 certificate and private key
//   - Requires clients to present valid certificates
//   - Verifies client certificates against a trusted CA
//   - Rejects TLS versions below 1.3
//
// This configuration ensures that only trusted, authenticated clients
// (e.g. Leader or Member nodes) can establish a connection.

func NewMTLSServerCreds(
	certFile, keyFile, caFile string,
) (credentials.TransportCredentials, error) {

	// ---------------------------------------------------------------------------
	// SERVER CERTIFICATE LOADING
	// ---------------------------------------------------------------------------
	//
	// Load the server's X.509 certificate and private key.
	// These identify the server during the TLS handshake.

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	// ---------------------------------------------------------------------------
	// CA CERTIFICATE LOADING
	// ---------------------------------------------------------------------------
	//
	// Read the trusted Certificate Authority (CA) certificate.
	// This CA is used to validate incoming client certificates.

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	// ---------------------------------------------------------------------------
	// CA CERTIFICATE POOL
	// ---------------------------------------------------------------------------
	//
	// Build a certificate pool containing the trusted CA.
	// Client certificates must chain back to this CA.

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	// ---------------------------------------------------------------------------
	// TLS CONFIGURATION
	// ---------------------------------------------------------------------------
	//
	// Server TLS behavior:
	//   - Present server certificate
	//   - Require AND verify client certificates
	//   - Enforce TLS 1.3 only

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}

// ===================================================================================
// CLIENT-SIDE mTLS CREDENTIALS
// ===================================================================================
//
// NewMTLSClientCreds creates TransportCredentials for a gRPC client that:
//
//   - Presents its own X.509 certificate to the server
//   - Verifies the server certificate using a trusted CA
//   - Validates server identity using ServerName (SAN / CN)
//   - Rejects TLS versions below 1.3
//
// This ensures that the client only connects to the intended server
// and not an impersonator.

func NewMTLSClientCreds(
	certFile, keyFile, caFile, serverName string,
) (credentials.TransportCredentials, error) {

	// ---------------------------------------------------------------------------
	// CLIENT CERTIFICATE LOADING
	// ---------------------------------------------------------------------------
	//
	// Load the client's X.509 certificate and private key.
	// These identify the client during the TLS handshake.

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	// ---------------------------------------------------------------------------
	// CA CERTIFICATE LOADING
	// ---------------------------------------------------------------------------
	//
	// Read the trusted Certificate Authority (CA) certificate.
	// This CA is used to validate the server certificate.

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	// ---------------------------------------------------------------------------
	// CA CERTIFICATE POOL
	// ---------------------------------------------------------------------------
	//
	// Build a certificate pool used to verify the server's certificate chain.

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	// ---------------------------------------------------------------------------
	// TLS CONFIGURATION
	// ---------------------------------------------------------------------------
	//
	// Client TLS behavior:
	//   - Present client certificate
	//   - Verify server certificate chain
	//   - Validate server identity via ServerName
	//   - Enforce TLS 1.3 only

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}
