package ossh

import (
	"bufio"
	"errors"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	gobrex "github.com/kujtimiihoxha/go-brace-expansion"
	"github.com/pborman/getopt/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// we need to define new type because by default getopt will split
// string arguments for the string lists using ',' as a delimiter
// New type should implement getopt.Value interface

type arrayFlags []string

func (i *arrayFlags) String() string {
	return ""
}

func (i *arrayFlags) Set(value string, option getopt.Option) error {
	*i = append(*i, value)
	return nil
}

// OsshSettings ...
type OsshSettings struct {
	HostStrings    arrayFlags
	CommandStrings arrayFlags
	HostFiles      arrayFlags
	CommandFiles   arrayFlags
	InventoryPath  string
	InventoryList  arrayFlags
	Logname        *string
	Key            *string
	Par            *int
	Preconnect     *bool
	ShowIP         *bool
	IgnoreFailures *bool
	Port           *int
	ConnectTimeout *int
	RunTimeout     *int
	Password       string
	MaxLabelLength *int
}

func (s *OsshSettings) ParseCliOptions() {
	var err error
	s.Logname = getopt.StringLong("user", 'u', os.Getenv("LOGNAME"), "Username for connections", "USER")
	s.Key = getopt.StringLong("key", 'k', "", "Use this private key", "PRIVATE_KEY")
	optHelp := getopt.BoolLong("help", '?', "Show help")
	getopt.FlagLong(&(s.HostStrings), "host", 'H', "Add the given HOST_STRING to the list of hosts", "HOST_STRING")
	getopt.FlagLong(&(s.HostFiles), "hosts", 'h', "Read hosts from file", "HOST_FILE")
	getopt.FlagLong(&(s.CommandStrings), "command", 'c', "Command to run", "COMMAND")
	getopt.FlagLong(&(s.CommandFiles), "command-file", 'C', "file with commands to run", "COMMAND_FILE")
	s.Par = getopt.IntLong("par", 'p', 512, "How many hosts to run simultaneously", "PARALLELISM")
	s.Preconnect = getopt.BoolLong("preconnect", 'P', "Connect to all hosts before running command")
	s.IgnoreFailures = getopt.BoolLong("ignore-failures", 'i', "Ignore connection failures in the preconnect mode")
	verbose = getopt.BoolLong("verbose", 'v', "Verbose output")
	s.Port = getopt.IntLong("port", 'o', 22, "Port to connect to", "PORT")
	s.ConnectTimeout = getopt.IntLong("connect-timeout", 'T', 60, "Connect timeout in seconds", "TIMEOUT")
	s.RunTimeout = getopt.IntLong("timeout", 't', 0, "Run timeout in seconds", "TIMEOUT")
	askpass := getopt.BoolLong("askpass", 'A', "Prompt for a password for ssh connects")
	s.ShowIP = getopt.BoolLong("showip", 'n', "In the output show ips instead of names")
	if s.InventoryPath, err = exec.LookPath("ossh-inventory"); err == nil {
		getopt.FlagLong(&(s.InventoryList), "inventory", 'I', "Use FILTER expression to select hosts from inventory", "FILTER")
	}
	getopt.Parse()

	if *optHelp {
		getopt.Usage()
		os.Exit(0)
	}
	if *askpass {
		s.Password = string(readBytePasswordFromTerminal("SSH password:"))
	}
}

func (s *OsshSettings) GetCommand() (string, error) {
	var out []string
	for _, commandFile := range s.CommandFiles {
		file, err := os.Open(commandFile)
		if err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(file)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line := scanner.Text()
			out = append(out, line)
		}
		defer file.Close()
	}
	out = append(out, strings.Join(s.CommandStrings, "\n"))
	return strings.Join(out, "\n"), nil
}

func (s *OsshSettings) getHost(address string, label string) (*OsshHost, error) {
	var err error
	hostPort := *(s.Port)
	addressAndPort := strings.Split(address, ":")
	hostAddress := addressAndPort[0]
	if len(addressAndPort) > 1 {
		if hostPort, err = strconv.Atoi(addressAndPort[1]); err != nil {
			return nil, err
		}
	}
	host := OsshHost{
		address:        hostAddress,
		label:          label,
		port:           hostPort,
		err:            nil,
		connectTimeout: time.Duration(*(s.ConnectTimeout)) * time.Second,
		runTimeout:     time.Duration(*(s.RunTimeout)) * time.Second,
	}
	err = host.setLabel(*s.ShowIP)
	if err != nil {
		return nil, err
	}
	if len(host.label) > *s.MaxLabelLength {
		*s.MaxLabelLength = len(host.label)
	}
	return &host, nil
}

func (s *OsshSettings) getInventoryHosts(hosts []OsshHost) ([]OsshHost, error) {
	if len(s.InventoryList) > 0 {
		var out []byte
		var err error
		var newHost *OsshHost
		if out, err = exec.Command(s.InventoryPath, s.InventoryList...).Output(); err != nil {
			return nil, err
		}
		for _, h := range strings.Split(string(out), "\n") {
			host := strings.Split(h, " ")
			if len(host) < 2 {
				continue
			}
			if newHost, err = s.getHost(host[1], host[0]); err != nil {
				return nil, err
			}
			hosts = append(hosts, *newHost)
		}
	}
	return hosts, nil
}

func (s *OsshSettings) processHostFiles() error {
	for _, hostFile := range s.HostFiles {
		file, err := os.Open(hostFile)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") {
				continue
			}
			s.HostStrings = append(s.HostStrings, line)
		}
		defer file.Close()
	}
	return nil
}

func (s *OsshSettings) processHostStrings(hosts []OsshHost) ([]OsshHost, error) {
	var err error
	var newHost *OsshHost
	for _, hostString := range s.HostStrings {
		for _, hs := range strings.Split(hostString, " ") {
			for _, h := range gobrex.Expand(hs) {
				if newHost, err = s.getHost(h, ""); err != nil {
					return nil, err
				}
				hosts = append(hosts, *newHost)
			}
		}
	}
	return hosts, nil
}

func (s *OsshSettings) GetHosts() ([]OsshHost, error) {
	var hosts []OsshHost
	var err error
	s.MaxLabelLength = new(int)
	hosts, err = s.getInventoryHosts(hosts)
	if err != nil {
		return nil, err
	}
	err = s.processHostFiles()
	if err != nil {
		return nil, err
	}
	hosts, err = s.processHostStrings(hosts)
	if err != nil {
		return nil, err
	}

	useColor := term.IsTerminal(int(os.Stdout.Fd()))
	// add space padding to the labels for better output formatting
	for i := 0; i < len(hosts); i++ {
		hosts[i].label = hosts[i].label + strings.Repeat(" ", *s.MaxLabelLength-len(hosts[i].label))
		hosts[i].useColor = useColor
	}
	return hosts, nil
}

func (s *OsshSettings) GetSSHClientConfig() (*ssh.ClientConfig, error) {
	var authMethod []ssh.AuthMethod
	if len(s.Password) > 0 {
		authMethod = append(authMethod, ssh.Password(s.Password))
	}
	if len(*s.Key) != 0 {
		publicKeyFile, err := publicKeyFile(*s.Key)
		if err != nil {
			return nil, err
		}
		authMethod = append(authMethod, publicKeyFile)
	}
	// ssh-agent has a UNIX socket under $SSH_AUTH_SOCK
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		// Use a callback rather than PublicKeys so we only consult the agent
		// once the remote server wants it.
		authMethod = append(authMethod, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}
	if len(authMethod) == 0 {
		return nil, errors.New("no authentication method provided")
	}
	return &ssh.ClientConfig{
		User:            *s.Logname,
		Auth:            authMethod,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}, nil
}
