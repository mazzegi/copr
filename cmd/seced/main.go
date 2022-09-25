package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mazzegi/copr"
	"github.com/mazzegi/copr/secrets"
	"github.com/pkg/errors"
	"golang.org/x/term"
)

func editor() string {
	return "nano"
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	secretFile := copr.SecretFile
	if len(os.Args) > 1 {
		secretFile = os.Args[1]
	}

	if _, err := os.Stat(secretFile); err != nil {
		fmt.Printf("Theres no secret file in this directory. Continue to create a new one\n")
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

	//create temp file
	tmpFile := ".secrets.tmp"
	err = os.WriteFile(tmpFile, sf.Data(), os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}
	defer func() {
		os.Remove(tmpFile)
	}()

	//call editor
	cmd := exec.Command(editor(), tmpFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "run editor %q", editor())
	}

	//
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return errors.Wrap(err, "read temp-file")
	}
	sf.Set(data)

	err = sf.Save()
	if err != nil {
		return errors.Wrap(err, "save scret file")
	}
	return nil
}
