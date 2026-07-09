package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/cli"
	"github.com/tidbcloud/tdc/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := cli.NewRootCommand(version.Current())
	if err := cli.Execute(ctx, root, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "tdc [ERROR]: %s\n", apperr.MessageFor(err))
		os.Exit(apperr.ExitCodeFor(err))
	}
}
