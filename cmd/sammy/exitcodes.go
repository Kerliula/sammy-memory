package main

// Exit codes returned by run and passed to os.Exit in main.
const (
	exitSuccess = 0 // The command completed successfully.
	exitFailure = 1 // The command failed while doing its work.
	exitUsage   = 2 // The command was missing or not recognized.
)
