package main

import (
	"os"
	"os/exec"
)

func main() {
	// Simply execute the actual main program
	cmd := exec.Command("go", "run", "cmd/server/main.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}