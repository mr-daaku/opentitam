package main

import (
	"github.com/opentitam-ai/opentitam/cmd"
	"github.com/opentitam-ai/opentitam/internal/logging"
)

func main() {
	defer logging.RecoverPanic("main", func() {
		logging.ErrorPersist("Application terminated due to unhandled panic")
	})

	cmd.Execute()
}
