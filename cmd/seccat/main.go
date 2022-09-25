package main

import (
	"fmt"
	"os"

	"github.com/mazzegi/copr/secrets"
	"github.com/pkg/errors"
	"golang.org/x/term"
)

const (
	secretFile = "copr.secrets"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(secretFile); err != nil {
		return errors.Errorf("Theres no secret file in this directory.")
	}

	fmt.Printf("Enter password: ")
	pwd, err := term.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, "read-password")
	}
	fmt.Println()
	sf, err := secrets.LoadFile(secretFile, string(pwd))
	if err != nil {
		return errors.Wrap(err, "load secrets file")
	}

	fmt.Printf("%s\n", sf.String())

	return nil
}
