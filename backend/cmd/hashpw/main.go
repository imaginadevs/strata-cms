// Throwaway helper: prints a bcrypt hash for a password passed as an argument.
// Usage: go run ./cmd/hashpw 'YourNewStrongPass'
// Delete this directory when you're done.
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/hashpw '<password>'")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash error:", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}
