package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Options struct {
	Profile string
	Debug   bool
	Output  string
	Query   string
}

func NewRootCommand(info version.Info) *cobra.Command {
	opts := &Options{}

	root := newCommand(commandSpec{
		Use:   "tdc",
		Short: "Agent-friendly CLI for TiDB Cloud Starter.",
		Long:  "tdc is an agent-friendly command line interface for TiDB Cloud Starter.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}, info)

	root.SilenceErrors = true
	root.SilenceUsage = true
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(flagErrorFunc)
	root.SetHelpTemplate(helpTemplate)
	root.SetUsageTemplate(usageTemplate)

	flags := root.PersistentFlags()
	flags.StringVar(&opts.Profile, "profile", "default", "profile name")
	flags.BoolVar(&opts.Debug, "debug", false, "enable debug output")
	flags.StringVar(&opts.Output, "output", "json", "output format")
	flags.StringVar(&opts.Query, "query", "", "JMESPath query applied to JSON output")

	root.AddCommand(newConfigureCommand(info))
	root.AddCommand(newCLICommand(info))
	root.AddCommand(newDBCommand(info))
	root.AddCommand(newFSCommand(info))
	root.AddCommand(newOrganizationCommand(info))

	installHelpCommands(root, info)
	applyCommandDefaults(root, info)

	return root
}

func Execute(ctx context.Context, root *cobra.Command, args []string, stdout, stderr io.Writer) error {
	if err := rejectShortFlags(args); err != nil {
		return err
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	_, err := root.ExecuteContextC(ctx)
	if err == nil {
		return nil
	}
	return normalizeError(err)
}

func rejectShortFlags(args []string) error {
	for _, arg := range args {
		if arg == "--" {
			return nil
		}
		if len(arg) > 1 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return apperr.New(
				"cli.short_flag_not_allowed",
				"usage",
				2,
				fmt.Sprintf("short flags are not supported: %s; use long flags such as --help", arg),
			)
		}
	}
	return nil
}

type commandSpec struct {
	Use     string
	Aliases []string
	Short   string
	Long    string
	RunE    func(cmd *cobra.Command, args []string) error
}

func newCommand(spec commandSpec, info version.Info) *cobra.Command {
	cmd := &cobra.Command{
		Use:           spec.Use,
		Aliases:       spec.Aliases,
		Short:         spec.Short,
		Long:          spec.Long,
		Version:       info.String(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          spec.RunE,
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	return cmd
}

func newParentCommand(use, short string, info version.Info) *cobra.Command {
	return newCommand(commandSpec{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}, info)
}

func newPlaceholderCommand(use, short string, info version.Info) *cobra.Command {
	return newCommand(commandSpec{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return apperr.NotImplemented(cmd.CommandPath())
		},
	}, info)
}

func applyCommandDefaults(cmd *cobra.Command, info version.Info) {
	cmd.Version = info.String()
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.SetFlagErrorFunc(flagErrorFunc)
	cmd.SetHelpTemplate(helpTemplate)
	cmd.SetUsageTemplate(usageTemplate)
	if cmd.Flags().Lookup("help") == nil {
		cmd.Flags().Bool("help", false, "help for this command")
	}
	if cmd.Flags().Lookup("version") == nil {
		cmd.Flags().Bool("version", false, "version for this command")
	}
	cmd.Flags().SortFlags = true
	cmd.PersistentFlags().SortFlags = true

	for _, child := range cmd.Commands() {
		applyCommandDefaults(child, info)
	}
}

func flagErrorFunc(cmd *cobra.Command, err error) error {
	return apperr.Wrap(
		"cli.invalid_flag",
		"usage",
		2,
		fmt.Sprintf("invalid flag for %s", cmd.CommandPath()),
		err,
	)
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return apperr.New("cli.interrupted", "runtime", 130, "interrupted")
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "unknown command"):
		return apperr.Wrap("cli.unknown_command", "usage", 2, msg, err)
	case strings.Contains(msg, "unknown flag"):
		return apperr.Wrap("cli.unknown_flag", "usage", 2, msg, err)
	case strings.Contains(msg, "accepts 0 arg"):
		return apperr.Wrap("cli.invalid_args", "usage", 2, msg, err)
	default:
		return apperr.Wrap("cli.execution_failed", "runtime", 1, msg, err)
	}
}

func HasShorthand(cmd *cobra.Command) bool {
	found := false
	visitCommands(cmd, func(current *cobra.Command) {
		for _, set := range []*pflag.FlagSet{
			current.Flags(),
			current.PersistentFlags(),
			current.LocalNonPersistentFlags(),
			current.InheritedFlags(),
		} {
			set.VisitAll(func(flag *pflag.Flag) {
				if flag.Shorthand != "" {
					found = true
				}
			})
		}
	})
	return found
}

func visitCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, child := range cmd.Commands() {
		visitCommands(child, fn)
	}
}

func installHelpCommands(cmd *cobra.Command, info version.Info) {
	for _, child := range cmd.Commands() {
		installHelpCommands(child, info)
	}
	if cmd.Name() == "help" || findChildCommand(cmd, "help") != nil {
		return
	}

	helpCmd := newCommand(commandSpec{
		Use:   "help",
		Short: "Help about this command.",
		RunE: func(help *cobra.Command, _ []string) error {
			return help.Parent().Help()
		},
	}, info)
	cmd.SetHelpCommand(helpCmd)
	cmd.AddCommand(helpCmd)
}

func findChildCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, child := range cmd.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

const helpTemplate = `{{with (or .Long .Short)}}{{.}}

{{end}}Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}

Commands:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`

const usageTemplate = `Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} help [command]{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`
