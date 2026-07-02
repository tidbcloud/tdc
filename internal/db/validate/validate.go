package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Icemap/tdc/internal/apperr"
)

const ClusterTypeStarter = "starter"

var clusterNamePattern = regexp.MustCompile(`^[A-Za-z0-9][-A-Za-z0-9]{2,62}[A-Za-z0-9]$`)

func Required(flagName, value string) error {
	if strings.TrimSpace(value) == "" {
		return apperr.New(
			"db.missing_required_flag",
			"usage",
			2,
			fmt.Sprintf("%s is required", flagName),
		)
	}
	return nil
}

func ClusterType(value string) error {
	if err := Required("--db-cluster-type", value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != ClusterTypeStarter {
		return apperr.New(
			"db.unsupported_cluster_type",
			"usage",
			2,
			"--db-cluster-type must be starter",
		)
	}
	return nil
}

func ClusterName(value string) error {
	if err := Required("--db-cluster-name", value); err != nil {
		return err
	}
	if !clusterNamePattern.MatchString(value) {
		return apperr.New(
			"db.invalid_cluster_name",
			"usage",
			2,
			"--db-cluster-name must be 4-64 characters, start and end with a letter or number, and contain only letters, numbers, and hyphens",
		)
	}
	return nil
}

func OptionalClusterName(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return ClusterName(value)
}

func ClusterID(value string) (string, error) {
	if err := Required("--db-cluster-id", value); err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "clusters/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", apperr.New(
			"db.invalid_cluster_id",
			"usage",
			2,
			"--db-cluster-id must be a TiDB Cloud cluster id, optionally prefixed with clusters/",
		)
	}
	return trimmed, nil
}

func View(value string) error {
	switch strings.TrimSpace(value) {
	case "", "BASIC", "FULL":
		return nil
	default:
		return apperr.New(
			"db.invalid_view",
			"usage",
			2,
			"--view must be BASIC or FULL",
		)
	}
}

func NonNegative(flagName string, value int32) error {
	if value < 0 {
		return apperr.New(
			"db.invalid_non_negative_flag",
			"usage",
			2,
			fmt.Sprintf("%s must be greater than or equal to 0", flagName),
		)
	}
	return nil
}

func OptionalNonNegative(flagName string, value int32) error {
	if value == -1 {
		return nil
	}
	return NonNegative(flagName, value)
}
