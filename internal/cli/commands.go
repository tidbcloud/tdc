package cli

import (
	"time"

	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
	cfgconfigure "github.com/Icemap/tdc/internal/config/configure"
	"github.com/Icemap/tdc/internal/db"
	"github.com/Icemap/tdc/internal/db/connectionstring"
	"github.com/Icemap/tdc/internal/dryrun"
	"github.com/Icemap/tdc/internal/organization"
	outputpkg "github.com/Icemap/tdc/internal/output"
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
		newDBCreateBranchCommand(info),
		newDBListBranchesCommand(info),
		newDBDescribeBranchCommand(info),
		newDBDeleteBranchCommand(info),
		newDBPrepareQueryAccessCommand(info),
		newDBCreateConnectionStringCommand(info),
		newDBExecuteSQLCommand(info),
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

func newDBCreateBranchCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "create-db-cluster-branch",
		Short:      "Create a Starter DB cluster branch.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterBranchCreate,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := createBranchOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.CreateBranch(ctx.cmd.Context(), opts)
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			opts, err := createBranchOptions(ctx, profile)
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunCreateBranch(ctx.cmd.Context(), ctx.CommandPath(), opts)
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("db-cluster-branch-name", "", "Starter DB cluster branch display name")
	return cmd
}

func newDBListBranchesCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "list-db-cluster-branches",
		Short:      "List Starter DB cluster branches.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterBranchRead,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			clusterID, err := ctx.StringFlag("db-cluster-id")
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
			return service.ListBranches(ctx.cmd.Context(), db.ListBranchesOptions{
				Profile:   profile,
				ClusterID: clusterID,
				PageSize:  pageSize,
				PageToken: pageToken,
			})
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().Int32("page-size", 0, "number of branches to request; 0 uses the API default")
	cmd.Flags().String("page-token", "", "page token returned by a previous list-db-cluster-branches call")
	return cmd
}

func newDBDescribeBranchCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "describe-db-cluster-branch",
		Short:      "Describe a Starter DB cluster branch.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterBranchRead,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			clusterID, err := ctx.StringFlag("db-cluster-id")
			if err != nil {
				return nil, err
			}
			branchID, err := ctx.StringFlag("db-cluster-branch-id")
			if err != nil {
				return nil, err
			}
			view, err := ctx.StringFlag("view")
			if err != nil {
				return nil, err
			}
			return service.DescribeBranch(ctx.cmd.Context(), db.DescribeBranchOptions{
				Profile:   profile,
				ClusterID: clusterID,
				BranchID:  branchID,
				View:      view,
			})
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("db-cluster-branch-id", "", "Starter DB cluster branch id")
	cmd.Flags().String("view", "", "detail level: BASIC or FULL")
	return cmd
}

func newDBDeleteBranchCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "delete-db-cluster-branch",
		Short:      "Delete a Starter DB cluster branch.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterBranchDelete,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := deleteBranchOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.DeleteBranch(ctx.cmd.Context(), opts)
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			opts, err := deleteBranchOptions(ctx, profile)
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunDeleteBranch(ctx.cmd.Context(), ctx.CommandPath(), opts)
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().String("db-cluster-branch-id", "", "Starter DB cluster branch id")
	cmd.Flags().String("confirm-db-cluster-branch-name", "", "required exact remote branch display name confirmation")
	return cmd
}

func newDBPrepareQueryAccessCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "prepare-db-query-access",
		Short:      "Prepare local SQL credentials for query execution.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterSQLUserCreate,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			clusterID, err := ctx.StringFlag("db-cluster-id")
			if err != nil {
				return nil, err
			}
			return service.PrepareQueryAccess(ctx.cmd.Context(), db.PrepareQueryAccessOptions{
				Profile:   profile,
				ClusterID: clusterID,
			})
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return dryrun.Result{}, err
			}
			clusterID, err := ctx.StringFlag("db-cluster-id")
			if err != nil {
				return dryrun.Result{}, err
			}
			return service.DryRunPrepareQueryAccess(ctx.cmd.Context(), ctx.CommandPath(), db.PrepareQueryAccessOptions{
				Profile:   profile,
				ClusterID: clusterID,
			})
		},
	}, info)
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	return cmd
}

func newDBCreateConnectionStringCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "create-db-connection-string",
		Short:      "Create a DB connection string from prepared credentials.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterSQLUserRead,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := connectionStringOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			result, err := service.CreateConnectionString(ctx.cmd.Context(), opts)
			if err != nil {
				return nil, err
			}
			if result.Format == connectionstring.FormatEnv {
				return outputpkg.Raw{Bytes: []byte(result.ConnectionString)}, nil
			}
			return result, nil
		},
	}, info)
	addSQLCredentialFlags(cmd)
	cmd.Flags().String("database", "", "database/default schema name")
	cmd.Flags().String("format", connectionstring.FormatMySQLURI, "connection string format: mysql-uri, jdbc, go-sql-driver, sqlalchemy, or env")
	cmd.Flags().String("env-prefix", "TIDB_", "dotenv variable prefix for --format env")
	cmd.Flags().Bool("env-include-database-url", false, "include a database URL variable with --format env")
	cmd.Flags().String("env-database-url-name", "DATABASE_URL", "database URL variable name for --format env")
	return cmd
}

func newDBExecuteSQLCommand(info version.Info) *cobra.Command {
	cmd := newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "execute-sql-statement",
		Short:      "Execute one SQL statement.",
		Mutation:   readOnlyCommand,
		Permission: authz.StarterSQLExecute,
		Run: func(ctx commandContext) (any, error) {
			service, profile, err := dbServiceAndProfile(ctx)
			if err != nil {
				return nil, err
			}
			opts, err := executeSQLOptions(ctx, profile)
			if err != nil {
				return nil, err
			}
			return service.ExecuteSQL(ctx.cmd.Context(), opts)
		},
	}, info)
	addSQLCredentialFlags(cmd)
	cmd.Flags().String("database", "", "database/default schema name")
	cmd.Flags().String("sql", "", "one SQL statement to execute")
	cmd.Flags().String("transport", "http", "SQL execution transport: http or mysql")
	return cmd
}

func addSQLCredentialFlags(cmd *cobra.Command) {
	cmd.Flags().String("db-cluster-id", "", "Starter DB cluster id")
	cmd.Flags().Bool("read-only", false, "use prepared read_only DB SQL credentials")
	cmd.Flags().Bool("read-write", false, "use prepared read_write DB SQL credentials")
	cmd.Flags().Bool("admin", false, "use prepared admin DB SQL credentials")
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

func createBranchOptions(ctx commandContext, profile *config.Profile) (db.CreateBranchOptions, error) {
	clusterID, err := ctx.StringFlag("db-cluster-id")
	if err != nil {
		return db.CreateBranchOptions{}, err
	}
	name, err := ctx.StringFlag("db-cluster-branch-name")
	if err != nil {
		return db.CreateBranchOptions{}, err
	}
	return db.CreateBranchOptions{
		Profile:     profile,
		ClusterID:   clusterID,
		DisplayName: name,
	}, nil
}

func deleteBranchOptions(ctx commandContext, profile *config.Profile) (db.DeleteBranchOptions, error) {
	clusterID, err := ctx.StringFlag("db-cluster-id")
	if err != nil {
		return db.DeleteBranchOptions{}, err
	}
	branchID, err := ctx.StringFlag("db-cluster-branch-id")
	if err != nil {
		return db.DeleteBranchOptions{}, err
	}
	confirmName, err := ctx.StringFlag("confirm-db-cluster-branch-name")
	if err != nil {
		return db.DeleteBranchOptions{}, err
	}
	return db.DeleteBranchOptions{
		Profile:                    profile,
		ClusterID:                  clusterID,
		BranchID:                   branchID,
		ConfirmDBClusterBranchName: confirmName,
	}, nil
}

func connectionStringOptions(ctx commandContext, profile *config.Profile) (db.CreateConnectionStringOptions, error) {
	common, err := sqlCommonOptions(ctx)
	if err != nil {
		return db.CreateConnectionStringOptions{}, err
	}
	format, err := ctx.StringFlag("format")
	if err != nil {
		return db.CreateConnectionStringOptions{}, err
	}
	envPrefix, err := ctx.StringFlag("env-prefix")
	if err != nil {
		return db.CreateConnectionStringOptions{}, err
	}
	envIncludeURL, err := ctx.BoolFlag("env-include-database-url")
	if err != nil {
		return db.CreateConnectionStringOptions{}, err
	}
	envURLName, err := ctx.StringFlag("env-database-url-name")
	if err != nil {
		return db.CreateConnectionStringOptions{}, err
	}
	return db.CreateConnectionStringOptions{
		Profile:                profile,
		ClusterID:              common.clusterID,
		Database:               common.database,
		ReadOnly:               common.readOnly,
		ReadWrite:              common.readWrite,
		Admin:                  common.admin,
		Format:                 format,
		EnvPrefix:              envPrefix,
		EnvIncludeDatabaseURL:  envIncludeURL,
		EnvDatabaseURLVariable: envURLName,
	}, nil
}

func executeSQLOptions(ctx commandContext, profile *config.Profile) (db.ExecuteSQLOptions, error) {
	common, err := sqlCommonOptions(ctx)
	if err != nil {
		return db.ExecuteSQLOptions{}, err
	}
	sql, err := ctx.StringFlag("sql")
	if err != nil {
		return db.ExecuteSQLOptions{}, err
	}
	transport, err := ctx.StringFlag("transport")
	if err != nil {
		return db.ExecuteSQLOptions{}, err
	}
	return db.ExecuteSQLOptions{
		Profile:   profile,
		ClusterID: common.clusterID,
		Database:  common.database,
		SQL:       sql,
		ReadOnly:  common.readOnly,
		ReadWrite: common.readWrite,
		Admin:     common.admin,
		Transport: transport,
	}, nil
}

type sqlCommon struct {
	clusterID string
	database  string
	readOnly  bool
	readWrite bool
	admin     bool
}

func sqlCommonOptions(ctx commandContext) (sqlCommon, error) {
	clusterID, err := ctx.StringFlag("db-cluster-id")
	if err != nil {
		return sqlCommon{}, err
	}
	database, err := ctx.StringFlag("database")
	if err != nil {
		return sqlCommon{}, err
	}
	readOnly, err := ctx.BoolFlag("read-only")
	if err != nil {
		return sqlCommon{}, err
	}
	readWrite, err := ctx.BoolFlag("read-write")
	if err != nil {
		return sqlCommon{}, err
	}
	admin, err := ctx.BoolFlag("admin")
	if err != nil {
		return sqlCommon{}, err
	}
	return sqlCommon{
		clusterID: clusterID,
		database:  database,
		readOnly:  readOnly,
		readWrite: readWrite,
		admin:     admin,
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
