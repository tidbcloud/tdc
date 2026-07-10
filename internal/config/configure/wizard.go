package configure

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/config/region"
	"github.com/tidbcloud/tdc/internal/config/store"
	"github.com/tidbcloud/tdc/internal/secretinput"
)

type Options struct {
	Profile        string
	HomeDir        string
	RegionCode     string
	TDCPublicKey   string
	TDCPrivateKey  string
	NonInteractive bool
	Env            map[string]string
	In             io.Reader
	Out            io.Writer
}

func Run(ctx context.Context, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Profile == "" {
		opts.Profile = config.DefaultProfile
	}
	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return apperr.Wrap("config.home_dir", "config", 1, "cannot determine home directory", err)
		}
		opts.HomeDir = home
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	input := opts.In
	if !secretinput.IsTerminal(opts.In) {
		input = bufio.NewReader(opts.In)
	}

	defaultRegion := region.DefaultPlacementCode()
	regionCode, err := valueOrPrompt(ctx, input, opts.Out, valueOrEnv(opts.RegionCode, opts.Env, "TDC_REGION_CODE"), "region code", fmt.Sprintf("Region code (%s): ", defaultRegion), defaultRegion, false, opts.NonInteractive)
	if err != nil {
		return err
	}
	placement, err := region.ParsePlacementCode(regionCode)
	if err != nil {
		return apperr.Wrap("config.invalid_region", "config", 2, err.Error(), err)
	}

	publicKey, err := valueOrPrompt(ctx, input, opts.Out, valueOrEnv(opts.TDCPublicKey, opts.Env, "TDC_PUBLIC_KEY"), "TiDB Cloud public key", "TiDB Cloud public key: ", "", false, opts.NonInteractive)
	if err != nil {
		return err
	}
	privateKey, err := valueOrPrompt(ctx, input, opts.Out, valueOrEnv(opts.TDCPrivateKey, opts.Env, "TDC_PRIVATE_KEY"), "TiDB Cloud private key", "TiDB Cloud private key: ", "", true, opts.NonInteractive)
	if err != nil {
		return err
	}

	if err := store.WriteProfile(opts.HomeDir, opts.Profile, store.ConfigProfile{
		RegionCode: placement.Code,
	}, store.CredentialsProfile{
		TDCPublicKey:  publicKey,
		TDCPrivateKey: privateKey,
	}); err != nil {
		return apperr.Wrap("config.write_failed", "config", 1, err.Error(), err)
	}

	_, _ = fmt.Fprintf(opts.Out, "Profile %q configured. Credentials saved to %s.\n", opts.Profile, store.CredentialsPath(opts.HomeDir))
	return nil
}

func valueOrPrompt(ctx context.Context, in io.Reader, out io.Writer, value, fieldName, prompt, defaultValue string, secret, nonInteractive bool) (string, error) {
	if value != "" {
		return strings.TrimSpace(value), nil
	}
	if nonInteractive {
		return "", apperr.New(
			"config.non_interactive_missing",
			"config",
			2,
			fmt.Sprintf("%s is required for non-interactive configure; provide a flag or TDC_* environment variable", fieldName),
		)
	}

	value, err := secretinput.Read(ctx, prompt, in, out, secret)
	if err != nil {
		return "", err
	}
	if value == "" {
		if defaultValue != "" {
			return defaultValue, nil
		}
		return "", apperr.New("config.required_input", "config", 2, fieldName+" is required")
	}
	return strings.TrimSpace(value), nil
}

func valueOrEnv(value string, env map[string]string, key string) string {
	if value != "" {
		return value
	}
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}
