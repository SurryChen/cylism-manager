package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/cylism/cylism-manager/api/proto/agent"
	agentSrv "github.com/cylism/cylism-manager/internal/agent"
	"google.golang.org/grpc"
)

var version = "1.0.0"

func main() {
	port := flag.Int("port", 9527, "gRPC listen port")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	agent.RegisterAgentServiceServer(srv, agentSrv.NewServer(version))

	log.Printf("Cylism Manager Agent v%s starting on port %d", version, *port)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
