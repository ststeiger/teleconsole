package clt

import (
	"flag"
	"fmt"
	"os"

	"github.com/gravitational/teleconsole/conf"
	"github.com/gravitational/teleconsole/lib"
	"github.com/gravitational/teleconsole/version"

	"github.com/gravitational/teleport/lib/auth/native"
	"github.com/gravitational/teleport/lib/client"
	teleport "github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
)

type App struct {
	// CLI arguments
	Args []string

	// Configuration (CLI flags plus confing file)
	conf *conf.Config

	// Fully configured API client for Teleconsole server
	client *APIClient
}

func (this *App) DebugDump() {
	log.Debugf("Server: %s, Args: %v",
		this.conf.APIEndpointURL,
		this.conf.Args)
}

func initLogging(verbosity int) {
	// our log:
	log.SetLevel(log.ErrorLevel)
	log.SetOutput(os.Stderr)

	// teleport log:
	utils.InitLoggerCLI()

	switch verbosity {
	case 1:
		// our log:
		log.SetLevel(log.InfoLevel)
	case 2:
		// our log:
		log.SetLevel(log.DebugLevel)
		// teleport log:
		utils.InitLoggerVerbose()
	case 3:
		// our log:
		log.SetLevel(log.DebugLevel)
		// teleport log:
		utils.InitLoggerDebug()
	}
}

// NewApp constructs and returns a "Teleconsole application object"
// initialized with the command line arguments, values from the
// configuration file, ready to run
func NewApp(fs *flag.FlagSet) (*App, error) {
	// if flags weren't given to us, create our own:
	if fs == nil {
		fs = flag.NewFlagSet("teleconsole", flag.ExitOnError)
	}
	// parse CLI flags
	verbose := fs.Bool("v", false, "")
	verbose2 := fs.Bool("vv", false, "")
	verbose3 := fs.Bool("vvv", false, "")
	runCommand := fs.String("c", "", "")
	serverFlag := fs.String("s", "", "")
	insecure := fs.Bool("insecure", false, "")
	forwardPorts := fs.String("L", "", "")
	forwardAddr := fs.String("f", "", "")

	fs.Usage = printHelp
	fs.Parse(os.Args[1:])
	cliArgs := fs.Args()

	// init logging:
	verbosity := 0
	if *verbose3 {
		verbosity = 3
	} else if *verbose2 {
		verbosity = 2
	} else if *verbose {
		verbosity = 1
	}
	initLogging(verbosity)

	// configure teleport internals to use our ping interval.
	// IMPORANT: these must be similar for proxies and servers
	teleport.SessionRefreshPeriod = SyncRefreshInterval
	teleport.ReverseTunnelAgentHeartbeatPeriod = SyncRefreshInterval * 2
	teleport.ServerHeartbeatTTL = SyncRefreshInterval * 2

	native.PrecalculatedKeysNum = 0

	// read configuration from rcfile in ~/
	config, err := conf.Get()
	if err != nil {
		log.Fatal("Configuration error: ", err)
	}
	// apply CLI flags to the config:
	if *serverFlag != "" {
		if err = config.SetServer(*serverFlag); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	// parse -L flag spec (forwarded ports)
	if *forwardPorts != "" {
		config.ForwardPorts, err = client.ParsePortForwardSpec([]string{*forwardPorts})
		if err != nil {
			return nil, err
		}
	}
	if *forwardAddr != "" {
		config.ForwardPort, err = lib.ParseForwardAddr(*forwardAddr)
		if err != nil {
			return nil, trace.Errorf("Invalid forwarding addres spec: %v\nExamples: localhost:5000 or http://gravitational.com", err)
		}
	}

	config.Verbosity = verbosity
	config.RunCommand = *runCommand
	config.Args = cliArgs
	config.InsecureHTTPS = *insecure

	return &App{
		Args:   cliArgs,
		conf:   config,
		client: NewAPIClient(config, version.Version),
	}, nil
}

func (this *App) Usage() {
	printHelp()
}

func (this *App) Join() error {
	if len(this.Args) == 0 {
		log.Fatal("Error: need an argument: session ID")
	}
	return Join(this.conf, this.client, this.Args[1])
}

func (this *App) Start() error {
	return StartBroadcast(this.conf, this.client, this.Args[0:])
}

func (this *App) GetConfig() *conf.Config {
	return this.conf
}

func printHelp() {
	fmt.Println(`Usage: teleconsole <flags> <command>

Teleconsole allows you to start a new shell session and invite your 
friends into it.

Simply close the session to stop sharing.

Flags:
   -f host:port  Invite joining parties to connect to host:port
   -L spec       Request port forwarding when joining an existing session
   -insecure     When set, the client will trust invalid SSL certifates
   -v            Verbose logging
   -vv           Extra verbose logging (debug mode)
   -s host:port  Teleconsole server address [teleconsole.com]

Commands:
    help               Print this help
    join [session-id]  Join active session

Examples:
  > teleconsole -f 5000  

    Starts a shared SSH session, also letting joining parties access TCP 
    port 5000 on your machine.

  > teleconsole -f gravitational.com:80

    Starts a shared SSH session, forwarding TCP port 80 to joining parties.
    They will be able to visit http://gravitational.com using your machine
    as a proxy.

  > teleconsole -L 5000:gravitational.com:80 join <session-id>

    Joins the existing session requesting to forward gravitational.com:80
    to local port 5000.

Made by Gravitational Inc http://gravitational.com`)
}
