package security

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"google.golang.org/grpc/credentials"
)

func NewMTLSServerCreds(
	certFile, keyFile, caFile string,
) (credentials.TransportCredentials, error) {

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}

func NewMTLSClientCreds(
	certFile, keyFile, caFile, serverName string,
) (credentials.TransportCredentials, error) {

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsConfig), nil
}
