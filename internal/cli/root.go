package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/auth"
	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
	"github.com/Icemap/tdc/internal/dryrun"
	"github.com/Icemap/tdc/internal/output"
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

type mutationMode string

const (
	readOnlyCommand mutationMode = "read_only"
	mutatingCommand mutationMode = "mutating"
)

type controlPlaneCommandSpec struct {
	Use        string
	Short      string
	Long       string
	Mutation   mutationMode
	Permission authz.Permission
	Run        func(commandContext) (any, error)
	DryRun     func(commandContext) (dryrun.Result, error)
}

type commandContext struct {
	cmd *cobra.Command
}

func (c commandContext) CommandPath() string {
	return c.cmd.CommandPath()
}

func (c commandContext) LoadProfile() (*config.Profile, error) {
	return loadProfileForCommand(c.cmd)
}

func (c commandContext) Permission() authz.Permission {
	permission, _ := authz.ForCommand(c.cmd.CommandPath())
	return permission
}

func (c commandContext) StringFlag(name string) (string, error) {
	return c.cmd.Flags().GetString(name)
}

func (c commandContext) BoolFlag(name string) (bool, error) {
	flag := c.cmd.Flag(name)
	if flag == nil {
		return false, nil
	}
	return strconv.ParseBool(flag.Value.String())
}

func (c commandContext) Int32Flag(name string) (int32, error) {
	return c.cmd.Flags().GetInt32(name)
}

func (c commandContext) Int64Flag(name string) (int64, error) {
	return c.cmd.Flags().GetInt64(name)
}

func (c commandContext) DurationFlag(name string) (time.Duration, error) {
	return c.cmd.Flags().GetDuration(name)
}

func newControlPlanePlaceholderCommand(use, short string, mutation mutationMode, permission authz.Permission, info version.Info) *cobra.Command {
	return newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        use,
		Short:      short,
		Mutation:   mutation,
		Permission: permission,
		Run: func(ctx commandContext) (any, error) {
			return nil, apperr.NotImplemented(ctx.CommandPath())
		},
	}, info)
}

func newControlPlaneCommand(spec controlPlaneCommandSpec, info version.Info) *cobra.Command {
	if spec.Permission == "" {
		panic(fmt.Sprintf("control-plane command %s must declare a permission", spec.Use))
	}
	cmd := newCommand(commandSpec{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := commandContext{cmd: cmd}
			if spec.Mutation == mutatingCommand {
				isDryRun, err := cmd.Flags().GetBool("dry-run")
				if err != nil {
					return err
				}
				if isDryRun {
					run := spec.DryRun
					if run == nil {
						run = defaultControlPlaneDryRun(spec)
					}
					result, err := run(ctx)
					if err != nil {
						return err
					}
					return renderStructured(cmd, result)
				}
			}

			run := spec.Run
			if run == nil {
				run = func(ctx commandContext) (any, error) {
					return nil, apperr.NotImplemented(ctx.CommandPath())
				}
			}
			result, err := run(ctx)
			if err != nil {
				return err
			}
			return renderStructured(cmd, result)
		},
	}, info)
	if spec.Mutation == mutatingCommand {
		cmd.Flags().Bool("dry-run", false, "validate the request without creating, updating, or deleting remote resources")
	}
	return cmd
}

func defaultControlPlaneDryRun(spec controlPlaneCommandSpec) func(commandContext) (dryrun.Result, error) {
	return func(ctx commandContext) (dryrun.Result, error) {
		profile, err := ctx.LoadProfile()
		if err != nil {
			return dryrun.Result{}, err
		}

		return dryrun.New(
			ctx.CommandPath(),
			strings.ReplaceAll(spec.Use, "-", "_"),
			dryrun.RequestSummary{
				Description: "service-specific request construction is pending implementation",
			},
			dryrun.Check{
				Name:    "config_and_credentials",
				Status:  "passed",
				Message: fmt.Sprintf("profile %q loaded", profile.Name),
			},
			dryrun.Check{
				Name:    "endpoint_selection",
				Status:  "passed",
				Message: fmt.Sprintf("%s %s", profile.CloudProvider, profile.RegionCode),
			},
			dryrun.Check{
				Name:    "permission_requirement",
				Status:  "passed",
				Message: string(spec.Permission),
			},
			dryrun.Check{
				Name:    "request_construction",
				Status:  "passed",
				Message: "shared dry-run envelope constructed; service-specific request shape is pending command implementation",
			},
		), nil
	}
}

func renderStructured(cmd *cobra.Command, result any) error {
	format, err := stringFlag(cmd, "output")
	if err != nil {
		return err
	}
	expression, err := stringFlag(cmd, "query")
	if err != nil {
		return err
	}
	return output.Render(cmd.OutOrStdout(), result, output.Options{
		Format: format,
		Query:  expression,
	})
}

func stringFlag(cmd *cobra.Command, name string) (string, error) {
	flag := cmd.Flag(name)
	if flag == nil {
		return "", nil
	}
	return flag.Value.String(), nil
}

func loadProfileForCommand(cmd *cobra.Command) (*config.Profile, error) {
	profileName, err := stringFlag(cmd, "profile")
	if err != nil {
		return nil, err
	}
	profileFlag := cmd.Flag("profile")
	profileExplicit := profileFlag != nil && profileFlag.Changed
	if profileExplicit {
		if strings.TrimSpace(profileName) == "" {
			return nil, apperr.New("config.empty_profile", "usage", 2, "--profile cannot be empty")
		}
	} else if envProfile := strings.TrimSpace(os.Getenv("TDC_PROFILE")); envProfile != "" {
		profileName = envProfile
		profileExplicit = true
	}
	return auth.LoadProfile(cmd.Context(), config.LoadOptions{
		Profile:         profileName,
		ProfileExplicit: profileExplicit,
	})
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
		fmt.Sprintf("invalid flag for %s: %v", cmd.CommandPath(), err),
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
	var converted interface {
		AppError() *apperr.Error
	}
	if errors.As(err, &converted) {
		if appErr := converted.AppError(); appErr != nil {
			return appErr
		}
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
