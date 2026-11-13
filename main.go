package main

import (
	"github.com/mr-daaku/opentitam/cmd"
	"github.com/mr-daaku/opentitam/internal/logging"
)

func main() {
	defer logging.RecoverPanic("main", func() {
		logging.ErrorPersist("Application terminated due to unhandled panic")
	})

	cmd.Execute()
}
