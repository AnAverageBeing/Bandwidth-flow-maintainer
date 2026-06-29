// bandwidth — CLI tool for the Docker bandwidth management daemon.
// Communicates with bandwidthd over a Unix socket at /var/run/bandwidth.sock.
package main

import (
	"fmt"
	"os"

	"github.com/AnAverageBeing/Bandwidth-flow-maintainer/internal/cli"
)

func main() {
	c := cli.NewCLI(cli.DefaultSocketPath())

	if len(os.Args) < 2 {
		c.Help()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "reapply":
		c.Reapply()
	case "reload":
		c.Reload()
	case "status":
		c.Status()
	case "doctor":
		c.Doctor()
	case "inspect":
		if len(args) > 0 {
			c.Inspect(args[0])
		} else {
			fmt.Println("Usage: bandwidth inspect <container-id>")
		}
	case "inspect-port":
		if len(args) > 0 {
			c.InspectPort(args[0])
		} else {
			fmt.Println("Usage: bandwidth inspect-port <port>")
		}
	case "reset":
		if len(args) > 0 {
			c.Reset(args[0])
		} else {
			fmt.Println("Usage: bandwidth reset <container|port|all>")
		}
	case "enable":
		c.Enable()
	case "disable":
		c.Disable()
	case "restart":
		c.Restart()
	case "stop":
		c.Stop()
	case "start":
		c.Start()
	case "logs":
		c.Logs()
	case "config":
		c.Config()
	case "configure", "setup":
		c.Configure()
	case "list":
		c.List()
	case "version":
		c.Version()
	case "health":
		c.Health()
	case "webhook":
		if len(args) > 0 && args[0] == "test" {
			c.WebhookTest()
		} else {
			fmt.Println("Usage: bandwidth webhook test")
		}
	case "export":
		format := "json"
		if len(args) > 0 {
			format = args[0]
		}
		c.Export(format)
	case "history":
		if len(args) > 0 {
			c.History(args[0])
		} else {
			fmt.Println("Usage: bandwidth history <container-id>")
		}
	case "cleanup":
		c.Cleanup()
	case "stats":
		c.Stats()
	case "limits":
		c.Limits()
	case "top":
		c.Top()
	case "daemon":
		c.Daemon()
	case "completion":
		shell := "bash"
		if len(args) > 0 {
			shell = args[0]
		}
		c.Completion(shell)
	case "help", "-h", "--help":
		c.Help()
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		c.Help()
		os.Exit(1)
	}
}
