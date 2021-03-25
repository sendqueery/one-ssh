package ossh

import (
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func readBytePasswordFromTerminal(prompt string) []byte {
	fmt.Printf("%s ", prompt)
	bytePassword, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Printf("\n")
	return bytePassword
}

func publicKeyFile(file string) (ssh.AuthMethod, error) {
	var key ssh.Signer
	var err error
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	_, rest := pem.Decode(buffer)
	if len(rest) > 0 {
		return nil, errors.New("Failed to decode the key \"" + file + "\"")
	}

	if key, err = ssh.ParsePrivateKey(buffer); err != nil {
		if err == err.(*ssh.PassphraseMissingError) {
			if key, err = ssh.ParsePrivateKeyWithPassphrase(buffer, readBytePasswordFromTerminal("SSH Passphrase:")); err != nil {
				return nil, err
			}
			return ssh.PublicKeys(key), nil
		}
		return nil, err
	}

	return ssh.PublicKeys(key), nil
}
