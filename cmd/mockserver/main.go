// Command mockserver runs the fake OpenAI-compatible endpoint standalone for
// manual smoke tests: `go run ./cmd/mockserver` prints the baseURL to use.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mr-daaku/opentitam/mockserver"
)

func main() {
	srv := mockserver.New(
		mockserver.Turn{Text: "Hello from OpenTitam mock! This is a streamed reply."},
		mockserver.Turn{Text: "Done — mock round trip OK."},
	)
	fmt.Println(srv.URL())
	os.Stdout.Sync()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	srv.Close()
}
