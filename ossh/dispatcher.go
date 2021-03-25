package ossh

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// OsshDisaptcher ...
type OsshDisaptcher struct {
	Par             int
	Command         string
	SSHClientConfig *ssh.ClientConfig
	Hosts           []OsshHost
	Preconnect      bool
	IgnoreFailures  bool
}

func (d *OsshDisaptcher) Validate() error {
	var errList []string
	if d.Par < 1 {
		errList = append(errList, "parallelism should be > 0")
	}
	if len(d.Command) == 0 {
		errList = append(errList, "no command is specified")
	}
	if len(d.Hosts) == 0 {
		errList = append(errList, "host list is empty")
	}
	if len(errList) > 0 {
		return errors.New(strings.Join(errList, "\n"))
	}
	return nil
}

func (d *OsshDisaptcher) Run() error {
	var failureCount int
	hostIdx := 0
	c := make(chan *OsshMessage)
	if d.Preconnect {
		for hostIdx = 0; hostIdx < len(d.Hosts); hostIdx++ {
			go (&d.Hosts[hostIdx]).sshConnect(c, d.SSHClientConfig)
		}
		for hostIdx = 0; hostIdx < len(d.Hosts); hostIdx++ {
			message, ok := <-c
			if !ok {
				return fmt.Errorf("channel got closed unexpectedly, exiting")
			}
			if (message.messageType & ERROR) != 0 {
				message.println()
				failureCount++
			} else if (message.messageType & VERBOSE) != 0 {
				message.println()
			}
		}
	}
	if !d.IgnoreFailures && failureCount > 0 {
		return fmt.Errorf("failed to connect to %d hosts, exiting", failureCount)
	}
	running := 0
	for hostIdx = 0; hostIdx < len(d.Hosts) && running < d.Par; hostIdx++ {
		if d.Hosts[hostIdx].err != nil {
			continue
		}
		go (&d.Hosts[hostIdx]).sshRun(c, d.SSHClientConfig, d.Command)
		running++
	}
	for running > 0 {
		message, ok := <-c
		if !ok {
			break
		}
		if (message.messageType & ERROR) != 0 {
			message.println()
			running--
		} else if (message.messageType & CLOSE) != 0 {
			running--
		} else {
			message.println()
			continue
		}
		if hostIdx < len(d.Hosts) {
			go (&d.Hosts[hostIdx]).sshRun(c, d.SSHClientConfig, d.Command)
			running++
			hostIdx++
		}
	}
	return nil
}
