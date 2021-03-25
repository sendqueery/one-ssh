package main

import (
	"log"

	"github.com/kt97679/one-ssh/ossh"
)

func main() {
	var err error
	settings := &ossh.OsshSettings{}
	settings.ParseCliOptions()
	dispatcher := &ossh.OsshDisaptcher{
		Par:            *settings.Par,
		IgnoreFailures: *settings.IgnoreFailures,
		Preconnect:     *settings.Preconnect,
	}

	if dispatcher.Command, err = settings.GetCommand(); err != nil {
		log.Fatal(err)
	}

	if dispatcher.SSHClientConfig, err = settings.GetSSHClientConfig(); err != nil {
		log.Fatal(err)
	}

	if dispatcher.Hosts, err = settings.GetHosts(); err != nil {
		log.Fatal(err)
	}

	if err = dispatcher.Validate(); err != nil {
		log.Fatal(err)
	}

	if err = dispatcher.Run(); err != nil {
		log.Fatal(err)
	}
}
