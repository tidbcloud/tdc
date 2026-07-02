package cli

import (
	"time"

	"github.com/Icemap/tdc/internal/authz"
	cfgconfigure "github.com/Icemap/tdc/internal/config/configure"
	"github.com/Icemap/tdc/internal/organization"
	"github.com/Icemap/tdc/internal/version"
	"github.com/spf13/cobra"
)

func newConfigureCommand(info version.Info) *cobra.Command {
	cmd := newCommand(commandSpec{
		Use:   "configure",
		Short: "Configure a local tdc profile.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, err := cmd.Flags().GetString("profile")
			if err != nil {
				return err
			}
			cloudProvider, err := cmd.Flags().GetString("cloud-provider")
			if err != nil {
				return err
			}
			regionCode, err := cmd.Flags().GetString("region-code")
			if err != nil {
				return err
			}
			publicKey, err := cmd.Flags().GetString("tdc-public-key")
			if err != nil {
				return err
			}
			privateKey, err := cmd.Flags().GetString("tdc-private-key")
			if err != nil {
				return err
			}
			nonInteractive, err := cmd.Flags().GetBool("non-interactive")
			if err != nil {
				return err
			}
			return cfgconfigure.Run(cmd.Context(), cfgconfigure.Options{
				Profile:        profile,
				CloudProvider:  cloudProvider,
				RegionCode:     regionCode,
				TDCPublicKey:   publicKey,
				TDCPrivateKey:  privateKey,
				NonInteractive: nonInteractive,
				In:             cmd.InOrStdin(),
				Out:            cmd.OutOrStdout(),
			})
		},
	}, info)
	cmd.Flags().String("cloud-provider", "", "cloud provider: aws or alibaba_cloud")
	cmd.Flags().String("region-code", "", "cloud region code")
	cmd.Flags().String("tdc-public-key", "", "TiDB Cloud public key")
	cmd.Flags().String("tdc-private-key", "", "TiDB Cloud private key; prefer TDC_PRIVATE_KEY for automation")
	cmd.Flags().Bool("non-interactive", false, "fail instead of prompting for missing configure values")
	return cmd
}

func newCLICommand(info version.Info) *cobra.Command {
	cmd := newParentCommand("cli", "Manage the tdc CLI installation.", info)
	cmd.AddCommand(
		newPlaceholderCommand("check-update", "Check whether a newer tdc release is available.", info),
		newPlaceholderCommand("update", "Update an owned tdc installation.", info),
	)
	return cmd
}

func newDBCommand(info version.Info) *cobra.Command {
	cmd := newParentCommand("db", "Manage TiDB Cloud Starter database resources.", info)
	cmd.AddCommand(
		newControlPlanePlaceholderCommand("create-db-cluster", "Create a Starter DB cluster.", mutatingCommand, authz.StarterClusterCreate, info),
		newControlPlanePlaceholderCommand("list-db-clusters", "List Starter DB clusters.", readOnlyCommand, authz.StarterClusterRead, info),
		newControlPlanePlaceholderCommand("describe-db-cluster", "Describe a Starter DB cluster.", readOnlyCommand, authz.StarterClusterRead, info),
		newControlPlanePlaceholderCommand("update-db-cluster", "Update a Starter DB cluster.", mutatingCommand, authz.StarterClusterUpdate, info),
		newControlPlanePlaceholderCommand("delete-db-cluster", "Delete a Starter DB cluster.", mutatingCommand, authz.StarterClusterDelete, info),
		newControlPlanePlaceholderCommand("create-db-cluster-branch", "Create a DB cluster branch.", mutatingCommand, authz.StarterBranchCreate, info),
		newControlPlanePlaceholderCommand("list-db-cluster-branches", "List DB cluster branches.", readOnlyCommand, authz.StarterBranchRead, info),
		newControlPlanePlaceholderCommand("describe-db-cluster-branch", "Describe a DB cluster branch.", readOnlyCommand, authz.StarterBranchRead, info),
		newControlPlanePlaceholderCommand("delete-db-cluster-branch", "Delete a DB cluster branch.", mutatingCommand, authz.StarterBranchDelete, info),
		newControlPlanePlaceholderCommand("prepare-db-query-access", "Prepare local SQL credentials for query execution.", mutatingCommand, authz.StarterSQLUserCreate, info),
		newControlPlanePlaceholderCommand("create-db-connection-string", "Create a DB connection string from prepared credentials.", readOnlyCommand, authz.StarterSQLUserRead, info),
		newControlPlanePlaceholderCommand("execute-sql-statement", "Execute one SQL statement.", readOnlyCommand, authz.StarterSQLExecute, info),
	)
	return cmd
}

func newFSCommand(info version.Info) *cobra.Command {
	cmd := newParentCommand("fs", "Manage and access tdc fs resources.", info)
	cmd.AddCommand(
		newControlPlanePlaceholderCommand("create-file-system", "Create a tdc fs resource.", mutatingCommand, authz.FSVolumeCreate, info),
		newControlPlanePlaceholderCommand("delete-file-system", "Delete a tdc fs resource.", mutatingCommand, authz.FSVolumeDelete, info),
		newControlPlanePlaceholderCommand("check-file-system", "Check tdc fs resource health.", readOnlyCommand, authz.FSVolumeRead, info),
		newPlaceholderCommand("copy-file", "Copy a file between local storage and tdc fs.", info),
		newPlaceholderCommand("read-file", "Read a file from tdc fs.", info),
		newPlaceholderCommand("list-files", "List files in tdc fs.", info),
		newPlaceholderCommand("describe-file", "Describe a tdc fs file.", info),
		newPlaceholderCommand("move-file", "Move a tdc fs file.", info),
		newPlaceholderCommand("delete-file", "Delete a tdc fs file.", info),
		newPlaceholderCommand("create-directory", "Create a tdc fs directory.", info),
		newPlaceholderCommand("search-file-content", "Search file content in tdc fs.", info),
		newPlaceholderCommand("find-files", "Find files in tdc fs.", info),
		newPlaceholderCommand("mount-file-system", "Mount a tdc fs resource locally.", info),
		newPlaceholderCommand("unmount-file-system", "Unmount a tdc fs resource.", info),
	)
	return cmd
}

func newOrganizationCommand(info version.Info) *cobra.Command {
	cmd := newParentCommand("organization", "Inspect TiDB Cloud organization resources.", info)
	cmd.AddCommand(
		newOrganizationListProjectsCommand(info),
	)
	return cmd
}

func newOrganizationListProjectsCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "list-projects",
		Short:      "List TiDB Cloud projects.",
		Mutation:   readOnlyCommand,
		Permission: authz.OrganizationProjectRead,
		Run: func(ctx commandContext) (any, error) {
			profile, err := ctx.LoadProfile()
			if err != nil {
				return nil, err
			}
			pageSize, err := ctx.Int32Flag("page-size")
			if err != nil {
				return nil, err
			}
			pageToken, err := ctx.StringFlag("page-token")
			if err != nil {
				return nil, err
			}
			debug, err := ctx.BoolFlag("debug")
			if err != nil {
				return nil, err
			}
			service := organization.Service{
				Timeout:     30 * time.Second,
				Debug:       debug,
				DebugWriter: ctx.cmd.ErrOrStderr(),
			}
			return service.ListProjects(ctx.cmd.Context(), organization.ListProjectsOptions{
				Profile:   profile,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
		},
	}, info)
	cmd.Flags().Int32("page-size", 0, "number of projects to request; 0 uses the API default")
	cmd.Flags().String("page-token", "", "page token returned by a previous list-projects call")
	return cmd
}
