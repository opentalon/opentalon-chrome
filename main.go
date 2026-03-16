package main

import (
	"fmt"
	"log"
	"net"
	"os"

	chromeplugin "github.com/opentalon/opentalon-chrome/plugin"
	pluginpkg "github.com/opentalon/opentalon/pkg/plugin"
)

func main() {
	handler := chromeplugin.NewHandler()

	// TCP mode: CHROME_GRPC_PORT=50051 → listen on TCP so Chrome and the plugin
	// can run as Docker sidecars while OpenTalon connects via grpc://.
	if port := os.Getenv("CHROME_GRPC_PORT"); port != "" {
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("opentalon-chrome: listen tcp :%s: %v", port, err)
		}
		hs := pluginpkg.Handshake{
			Version: pluginpkg.HandshakeVersion,
			Network: "tcp",
			Address: "0.0.0.0:" + port,
		}
		if _, err := fmt.Fprintln(os.Stdout, hs.String()); err != nil {
			log.Fatalf("opentalon-chrome: write handshake: %v", err)
		}
		if err := pluginpkg.ServeListener(ln, handler); err != nil {
			log.Fatalf("opentalon-chrome: serve: %v", err)
		}
		return
	}

	// Default: Unix socket mode (launched as subprocess by OpenTalon).
	if err := pluginpkg.Serve(handler); err != nil {
		log.Fatalf("opentalon-chrome: serve: %v", err)
	}
}
