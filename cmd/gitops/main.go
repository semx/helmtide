package main

import (
	"log"
	"os"

	"github.com/semx/helmtide/pkg/helper"

	"github.com/semx/helmtide/pkg/action"
	helmwave "github.com/semx/helmtide/pkg/version"
	"github.com/urfave/cli/v2"
)

func main() {
	helper.Dotenv()

	c := cli.NewApp()
	c.Usage = "just generates manifests"
	c.Commands = commands
	c.Version = helmwave.Version

	if err := c.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

var commands = []*cli.Command{
	new(action.Manifests).Cmd(),
}
