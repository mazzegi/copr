package main

import (
	"fmt"
	"os"

	"github.com/mazzegi/copr"
	"github.com/pkg/errors"
	"golang.org/x/term"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	secretFile := copr.SecretFile
	if _, err := os.Stat(secretFile); err != nil {
		fmt.Printf("Theres no secret file in this directory. Continue to create a new one\n")
	}

	fmt.Printf("Enter password: ")
	pwd, err := term.ReadPassword(0)
	if err != nil {
		return errors.Wrap(err, "read-password")
	}
	fmt.Println()

	cs, err := copr.NewSecrets(secretFile, string(pwd))
	if err != nil {
		return errors.Wrapf(err, "new-secrets at %q", secretFile)
	}

	subCmd := "show"
	if len(os.Args) > 1 {
		subCmd = os.Args[1]
	}

	switch subCmd {
	case "show":
		show(cs)
	case "set":
		if len(os.Args) < 4 {
			fmt.Printf("usage: coprsec set <key> <value>\n")
			return nil
		}
		err := set(cs, os.Args[2], os.Args[3])
		if err != nil {
			return errors.Wrap(err, "set")
		} else {
			show(cs)
		}
	case "del":
		if len(os.Args) < 3 {
			fmt.Printf("usage: coprsec del <key>\n")
			return nil
		}
		err := del(cs, os.Args[2])
		if err != nil {
			return errors.Wrap(err, "del")
		} else {
			show(cs)
		}
	default:
		return errors.Errorf("unknown sub-command %q", subCmd)
	}

	return nil
}

func show(cs *copr.Secrets) {
	for _, k := range cs.Keys() {
		if v, ok := cs.Find(k); ok {
			fmt.Printf("%q = %q\n", k, v)
		}
	}
}

func set(cs *copr.Secrets, key, val string) error {
	cs.Set(key, val)
	return cs.Save()
}

func del(cs *copr.Secrets, key string) error {
	cs.Delete(key)
	return cs.Save()
}
