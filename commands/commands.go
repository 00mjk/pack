package commands

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

//go:generate mockgen -package mocks -destination mocks/pack_client.go github.com/buildpack/pack/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool) (*pack.BuilderInfo, error)
	Rebase(context.Context, pack.RebaseOptions) error
	CreateBuilder(context.Context, pack.CreateBuilderOptions) error
}

type suggestedBuilder struct {
	name  string
	image string
}

var suggestedBuilders = [][]suggestedBuilder{
	{
		{"Cloud Foundry", "cloudfoundry/cnb:bionic"},
		{"Cloud Foundry", "cloudfoundry/cnb:cflinuxfs3"},
	},
	{
		{"Heroku", "heroku/buildpacks"},
	},
}

func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func logError(logger logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			if !IsSoftError(err) {
				logger.Error(err.Error())
			}
			return err
		}
		return nil
	}
}

func multiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}

func createCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}

func suggestSettingBuilder(logger logging.LoggerWithWriter, client PackClient) {
	out := logger.Writer()
	_, _ = fmt.Fprintln(out, "Please select a default builder with:")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out,"\tpack set-default-builder <builder image>")
	_, _ = fmt.Fprintln(out)
	suggestBuilders(logger, client)
}

func suggestBuilders(logger logging.LoggerWithWriter, client PackClient) {
	out := logger.Writer()
	_, _ = fmt.Fprintln(out, "Suggested builders:")
	_, _ = fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 10, 10, 5, ' ', tabwriter.TabIndent)
	for _, i := range rand.Perm(len(suggestedBuilders)) {
		builders := suggestedBuilders[i]
		for _, builder := range builders {
			_, _ = tw.Write([]byte(fmt.Sprintf("\t%s:\t%s\t%s\t\n", builder.name, style.Symbol(builder.image), getBuilderDescription(builder.image, client))))
		}
	}
	_ = tw.Flush()

	logger.Info("")
	logging.Tip(logger, "Learn more about a specific builder with:\n")
	logger.Info("\tpack inspect-builder [builder image]")
}

var defaultBuilderDescriptions = map[string]string{
	"cloudfoundry/cnb:bionic":     "Small base image with Java & Node.js",
	"cloudfoundry/cnb:cflinuxfs3": "Larger base image with Java, Node.js & Python",
	"heroku/buildpacks":           "heroku-18 base image with official Heroku buildpacks",
}

func getBuilderDescription(builderName string, client PackClient) string {
	desc := ""
	info, err := client.InspectBuilder(builderName, false)
	if err == nil {
		desc = info.Description
	}

	if desc == "" {
		defaultDesc, ok := defaultBuilderDescriptions[builderName]
		if ok {
			desc = defaultDesc
		}
	}

	return desc
}

func suggestStacks(logger *logging.Logger) {
	logger.Info(`
Stacks maintained by the Cloud Native Buildpacks project:

    Stack ID: io.buildpacks.stacks.bionic
    Description: Minimal Ubuntu 18.04 stack
    Maintainer: Cloud Native Buildpacks
    Build Image: cnbs/build:bionic
    Run Image: cnbs/run:bionic

Stacks maintained by the community:

    Stack ID: heroku-18
    Description: The official Heroku stack based on Ubuntu 18.04
    Maintainer: Heroku
    Build Image: heroku/pack:18-build
    Run Image: heroku/pack:18

    Stack ID: org.cloudfoundry.stacks.cflinuxfs3
    Description: The official Cloud Foundry stack based on Ubuntu 18.04
    Maintainer: Cloud Foundry
    Build Image: cfbuildpacks/cflinuxfs3-cnb-experimental:build
    Run Image: cfbuildpacks/cflinuxfs3-cnb-experimental:run`)
}
