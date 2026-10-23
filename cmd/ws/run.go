package ws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dont-be-evil-company/withsecrets/internal/config"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/log"
	"github.com/dont-be-evil-company/withsecrets/internal/lib/secrets"
	"github.com/spf13/cobra"
)

var (
	environment string
	configFile  string
	contain     bool
	commandFlag string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a command with secrets from cloud providers",
	Long: `Run a command with environment variables populated from secrets stored in cloud providers.
	
This command will:
1. Load the ws.yaml configuration file
2. Fetch secrets from the specified cloud providers
3. Set the secrets as environment variables
4. Execute the provided command with those environment variables

By default, secrets are merged with the current OS environment. Use --contain to only use
environment variables from ws.yaml.

Example:
  ws run -- node server.js
  ws run --env production -- python app.py
  ws run --config ./config/ws.yaml -- docker-compose up
  ws run --contain -- node server.js
  ws run --command 'echo "$SOME_SECRET"'`,
	Args: func(cmd *cobra.Command, args []string) error {
		// If --command is provided, args are optional
		if cmd.Flags().Changed("command") {
			return nil
		}
		// Otherwise, require at least one argument
		if len(args) < 1 {
			return fmt.Errorf("requires at least 1 arg(s), only received %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand(args)
	},
}

func init() {
	runCmd.Flags().StringVarP(&environment, "env", "e", "default", "Environment to use (default: default)")
	runCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to ws.yaml configuration file")
	runCmd.Flags().BoolVar(&contain, "contain", false, "Only use environment variables from ws.yaml, do not merge with OS environment")
	runCmd.Flags().StringVar(&commandFlag, "command", "", "Run an arbitrary command string in a shell with access to injected environment variables")
	rootCmd.AddCommand(runCmd)
}

func runCommand(args []string) error {
	logger := log.NewLogger()

	// Find configuration file if not specified
	if configFile == "" {
		var err error
		logger.Debug("No config file specified, searching for ws.yaml")
		configFile, err = config.FindConfigFile()
		if err != nil {
			return fmt.Errorf("failed to find configuration file: %w", err)
		}
		logger.Debug("Found configuration file", "path", configFile)
	} else {
		logger.Debug("Using specified configuration file", "path", configFile)
	}

	// Load configuration
	logger.Debug("Loading configuration from file")
	kubaConfig, err := config.LoadSecretsConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	logger.Debug("Configuration loaded successfully")

	// Get environment configuration
	logger.Debug("Getting environment configuration", "environment", environment)
	env, err := kubaConfig.GetEnvironment(environment)
	if err != nil {
		return fmt.Errorf("failed to get environment '%s': %w", environment, err)
	}
	logger.Debug("Environment configuration retrieved", "environment", environment, "provider", env.Provider, "env_count", len(env.Env))

	// Create secrets manager factory
	logger.Debug("Creating secrets manager factory")
	factory := secrets.NewSecretManagerFactory()

	// Get secrets for the environment
	ctx := context.Background()
	logger.Debug("Fetching secrets from cloud providers")
	secrets, _, err := factory.GetSecretsForEnvironmentWithCache(ctx, env, configFile, environment)
	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}
	logger.Debug("Secrets retrieved successfully", "count", len(secrets))

	// Prepare environment variables (used for both execution modes)
	var cmdEnv []string
	if contain {
		// Only use secrets from ws.yaml, do not merge with OS environment
		cmdEnv = make([]string, 0, len(secrets))
	} else {
		// Default behavior: merge OS environment with secrets
		cmdEnv = os.Environ()
	}
	for key, value := range secrets {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", key, value))
	}
	logger.Debug("Environment variables set", "secrets_count", len(secrets), "total_env_vars", len(cmdEnv))

	// Prepare command execution
	var cmd *exec.Cmd
	if commandFlag != "" {
		// Execute command string in a shell.
		//
		// On Unix, prefer $SHELL and fall back to "sh" (PATH-based).
		// On Windows, use COMSPEC/cmd.exe so we don't depend on /bin/sh.
		if runtime.GOOS == "windows" {
			shell := os.Getenv("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
			logger.Debug("Preparing shell command execution", "shell", shell, "command", commandFlag)
			cmd = exec.Command(shell, "/C", commandFlag)
		} else {
			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "sh"
			}
			logger.Debug("Preparing shell command execution", "shell", shell, "command", commandFlag)
			cmd = exec.Command(shell, "-c", commandFlag)
		}
	} else {
		// Execute command directly.
		//
		// Important: when the command is a bare name (e.g. "turbo"), Go resolves it
		// using the current process PATH, not cmd.Env. If kuba injects/overrides PATH
		// via secrets, `ws run --command ...` will see it (shell), but `ws run -- ...`
		// would fail to find the executable unless we resolve it using the final env.
		command := args[0]
		commandArgs := args[1:]
		resolvedCommand, err := lookPathWithEnv(command, cmdEnv)
		if err != nil {
			return fmt.Errorf("failed to find command %q in PATH: %w", command, err)
		}
		logger.Debug("Preparing command execution", "command", resolvedCommand, "args", commandArgs)
		cmd = exec.Command(resolvedCommand, commandArgs...)
	}
	cmd.Env = cmdEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute command
	logger.Debug("Executing command")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Debug("Command exited with non-zero status", "exit_code", exitErr.ExitCode())
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("command failed: %w", err)
	}

	logger.Debug("Command executed successfully")
	return nil
}

func lookPathWithEnv(file string, env []string) (string, error) {
	// If the user provided an explicit path, just use it.
	if strings.ContainsRune(file, os.PathSeparator) || (runtime.GOOS == "windows" && strings.Contains(file, `\`)) {
		return file, nil
	}

	pathVal := getEnvVar(env, "PATH")
	if pathVal == "" {
		pathVal = os.Getenv("PATH")
	}

	if runtime.GOOS == "windows" {
		return lookPathWithEnvWindows(file, pathVal, getEnvVar(env, "PATHEXT"))
	}

	for _, dir := range filepath.SplitList(pathVal) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, file)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func getEnvVar(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// Windows doesn't use POSIX executable bits; existence + extension is what matters.
	// Extension handling is done by lookPathWithEnvWindows via PATHEXT, so here we just
	// require the file to exist and not be a directory.
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func lookPathWithEnvWindows(file, pathVal, pathext string) (string, error) {
	// Minimal Windows lookup that respects PATH and PATHEXT.
	exts := []string{""}
	if filepath.Ext(file) == "" {
		if pathext == "" {
			pathext = os.Getenv("PATHEXT")
		}
		if pathext != "" {
			exts = []string{}
			for _, e := range strings.Split(pathext, ";") {
				e = strings.TrimSpace(e)
				if e == "" {
					continue
				}
				exts = append(exts, strings.ToLower(e))
			}
		}
	}

	for _, dir := range filepath.SplitList(pathVal) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			candidate := filepath.Join(dir, file)
			if ext != "" && strings.ToLower(filepath.Ext(candidate)) != ext {
				candidate += ext
			}
			if isExecutableFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}
