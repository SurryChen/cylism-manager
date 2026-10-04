package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cylism/cylism-manager/internal/clusteragent"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "csr" {
		runCSR(os.Args[2:])
		return
	}
	args := os.Args[1:]
	// The systemd unit uses an explicit "serve" subcommand. Keep the
	// default invocation compatible while consuming that subcommand before
	// passing flags to the standard flag parser.
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	runServe(args)
}

func runCSR(args []string) {
	f := flag.NewFlagSet("csr", flag.ExitOnError)
	hostname := f.String("hostname", "", "the control plane hostname or IP")
	key := f.String("key-path", "/etc/cylism/cluster-agent/agent.key", "private key path")
	csr := f.String("csr-path", "/etc/cylism/cluster-agent/agent.csr", "CSR path")
	_ = f.Parse(args)
	output, err := clusteragent.GenerateCSR(*hostname, *key, *csr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(output)
}

func runServe(args []string) {
	f := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := f.String("listen-address", "0.0.0.0:9443", "listen address")
	kubeAPI := f.String("kube-api", "https://127.0.0.1:6443", "local Kubernetes API URL")
	kubeConfig := f.String("kubeconfig", "/etc/rancher/k3s/k3s.yaml", "local Kubernetes kubeconfig")
	cert := f.String("server-cert", "/etc/cylism/cluster-agent/agent.crt", "server certificate")
	key := f.String("server-key", "/etc/cylism/cluster-agent/agent.key", "server private key")
	ca := f.String("client-ca", "/etc/cylism/cluster-agent/ca.crt", "client CA")
	clusterID := f.Uint("cluster-id", 0, "cluster identity")
	_ = f.Parse(args)
	if *clusterID == 0 {
		log.Fatal("cluster-id is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := clusteragent.Serve(ctx, clusteragent.Config{ListenAddress: *listen, KubeAPI: strings.TrimSpace(*kubeAPI), KubeConfig: *kubeConfig, ServerCert: *cert, ServerKey: *key, ClientCA: *ca, ClusterID: uint(*clusterID)})
	if err != nil {
		log.Fatal(err)
	}
}
