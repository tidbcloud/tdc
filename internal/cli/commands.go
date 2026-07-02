package cli

import (
	"time"

	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
	cfgconfigure "github.com/Icemap/tdc/internal/config/configure"
	"github.com/Icemap/tdc/internal/db"
	"github.com/Icemap/tdc/internal/dryrun"
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
		newDBCreateClusterCommand(info),
		newDBListClustersCommand(info),
		newDBDescribeClusterCommand(info),
		newDBUpdateClusterCommand(info),
		newDBDeleteClusterCommand(info),
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

func newDBCreateClusterCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "create-db-cluster",
		Short:      "Create a Starter DB cluster.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterClusterCreate,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := createClusterOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.CreateCluster(ctx.cmd.Context(), opts)
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			opts, err := createClusterOptions(ctx, profile)
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunCreateCluster(ctx.cmd.Context(), ctx.CommandPath(), opts)
		},
	}, info)
	cmd.Flags().String("db-cluster-name", "", "Starter DB cluster display name")
	cmd.Flags().String("db-cluster-type", "", "DB cluster type; must be starter")
	cmd.Flags().String("project-id", "", "TiDB Cloud project id")
	cmd.Flags().Int32("monthly-spending-limit-usd-cents", -1, "monthly spending limit in USD cents; omit to use the API default")
	return cmd
}

func newDBListClustersCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "list-db-clusters",
		Short:      "List Starter DB clusters.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterClusterRead,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
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
			filter, err := ctx.StringFlag("filter")
			if err != nil {
				return nil, err
			}
			orderBy, err := ctx.StringFlag("order-by")
			if err != nil {
				return nil, err
			}
			skip, err := ctx.Int32Flag("skip")
			if err != nil {
				return nil, err
			}
			return service.ListClusters(ctx.cmd.Context(), db.ListClustersOptions{
				Profile:   profile,
				PageSize:  pageSize,
				PageToken: pageToken,
				Filter:    filter,
				OrderBy:   orderBy,
				Skip:      skip,
			})
		},
	}, info)
	cmd.Flags().Int32("page-size", 0, "number of clusters to request; 0 uses the API default")
	cmd.Flags().String("page-token", "", "page token returned by a previous list-db-clusters call")
	cmd.Flags().String("filter", "", "Starter API filter expression")
	cmd.Flags().String("order-by", "", "Starter API orderBy expression")
	cmd.Flags().Int32("skip", 0, "number of clusters to skip")
	return cmd
}

func newDBDescribeClusterCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "describe-db-cluster",
		Short:      "Describe a Starter DB cluster.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterClusterRead,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			clusterID, err := ctx.StringFlag("db-cluster-id")
			if err != nil {
				return nil, err
			}
			view, err := ctx.StringFlag("view")
			if err != nil {
				return nil, err
			}
			return service.DescribeCluster(ctx.cmd.Context(), db.DescribeClusterOptions{
				Profile:   profile,
				ClusterID: clusterID,
				View:      view,
			})
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("view", "", "detail level: BASIC or FULL")
	return cmd
}

func newDBUpdateClusterCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "update-db-cluster",
		Short:      "Update a Starter DB cluster.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterClusterUpdate,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := updateClusterOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.UpdateCluster(ctx.cmd.Context(), opts)
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			opts, err := updateClusterOptions(ctx, profile)
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunUpdateCluster(ctx.cmd.Context(), ctx.CommandPath(), opts)
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("db-cluster-name", "", "new Starter DB cluster display name")
	cmd.Flags().Int32("monthly-spending-limit-usd-cents", -1, "monthly spending limit in USD cents; omit to leave unchanged")
	return cmd
}

func newDBDeleteClusterCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "delete-db-cluster",
		Short:      "Delete a Starter DB cluster.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterClusterDelete,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := deleteClusterOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.DeleteCluster(ctx.cmd.Context(), opts)
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			opts, err := deleteClusterOptions(ctx, profile)
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunDeleteCluster(ctx.cmd.Context(), ctx.CommandPath(), opts)
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("confirm-db-cluster-name", "", "required exact remote cluster display name confirmation")
	return cmd
}

func dbServiceAndProfile(ctx commandContext) (db.Service, *config.Profile, error) {
	profile, err := ctx.LoadProfile()
	if err != nil {
		return db.Service{}, nil, err
	}
	debug, err := ctx.BoolFlag("debug")
	if err != nil {
		return db.Service{}, nil, err
	}
	return db.Service{
		Timeout:     30 * time.Second,
		Debug:       debug,
		DebugWriter: ctx.cmd.ErrOrStderr(),
	}, profile, nil
}

func createClusterOptions(ctx commandContext, profile *config.Profile) (db.CreateClusterOptions, error) {
	name, err := ctx.StringFlag("db-cluster-name")
	if err != nil {
		return db.CreateClusterOptions{}, err
	}
	clusterType, err := ctx.StringFlag("db-cluster-type")
	if err != nil {
		return db.CreateClusterOptions{}, err
	}
	projectID, err := ctx.StringFlag("project-id")
	if err != nil {
		return db.CreateClusterOptions{}, err
	}
	spendingLimit, err := ctx.Int32Flag("monthly-spending-limit-usd-cents")
	if err != nil {
		return db.CreateClusterOptions{}, err
	}
	return db.CreateClusterOptions{
		Profile:                      profile,
		DisplayName:                  name,
		ClusterType:                  clusterType,
		ProjectID:                    projectID,
		MonthlySpendingLimitUSDCents: spendingLimit,
	}, nil
}

func updateClusterOptions(ctx commandContext, profile *config.Profile) (db.UpdateClusterOptions, error) {
	clusterID, err := ctx.StringFlag("db-cluster-id")
	if err != nil {
		return db.UpdateClusterOptions{}, err
	}
	name, err := ctx.StringFlag("db-cluster-name")
	if err != nil {
		return db.UpdateClusterOptions{}, err
	}
	spendingLimit, err := ctx.Int32Flag("monthly-spending-limit-usd-cents")
	if err != nil {
		return db.UpdateClusterOptions{}, err
	}
	return db.UpdateClusterOptions{
		Profile:                      profile,
		ClusterID:                    clusterID,
		DisplayName:                  name,
		MonthlySpendingLimitUSDCents: spendingLimit,
	}, nil
}

func deleteClusterOptions(ctx commandContext, profile *config.Profile) (db.DeleteClusterOptions, error) {
	clusterID, err := ctx.StringFlag("db-cluster-id")
	if err != nil {
		return db.DeleteClusterOptions{}, err
	}
	confirmName, err := ctx.StringFlag("confirm-db-cluster-name")
	if err != nil {
		return db.DeleteClusterOptions{}, err
	}
	return db.DeleteClusterOptions{
		Profile:              profile,
		ClusterID:            clusterID,
		ConfirmDBClusterName: confirmName,
	}, nil
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
