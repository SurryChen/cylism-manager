package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/cylism/cylism-manager/api/proto/agent"
	agentSrv "github.com/cylism/cylism-manager/internal/agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var version = "1.0.0"

func main() {
	port := flag.Int("port", 9527, "gRPC listen port")
	showVersion := flag.Bool("version", false, "print version and exit")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	tlsCA := flag.String("tls-ca", "", "TLS CA certificate file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("cylism-agent version %s\n", version)
		return
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	var opts []grpc.ServerOption

	if *tlsCert != "" && *tlsKey != "" && *tlsCA != "" {
		cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("Failed to load TLS cert: %v", err)
		}

		caPEM, err := os.ReadFile(*tlsCA)
		if err != nil {
			log.Fatalf("Failed to load CA cert: %v", err)
		}
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(caPEM)

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    caPool,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		log.Println("mTLS enabled")
	}

	srv := grpc.NewServer(opts...)
	agent.RegisterAgentServiceServer(srv, agentSrv.NewServer(version))

	log.Printf("Cylism Manager Agent v%s starting on port %d", version, *port)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
