<div align="center">

![withsecrets logo](assets/logo.svg)

# withsecrets

CLI: **`ws`**

[![Made with love](assets/badge-made-with-love.svg)](https://github.com/dont-be-evil-company/withsecrets/graphs/contributors)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/dont-be-evil-company/withsecrets?style=for-the-badge)](https://github.com/dont-be-evil-company/withsecrets/releases/latest)
[![Development status)](assets/badge-development-status.svg)](https://github.com/orgs/mistweaverco/projects/5/views/1?filterQuery=repo%3Adont-be-evil-company%2Fwithsecrets)
[![License](https://img.shields.io/github/license/dont-be-evil-company/withsecrets?style=for-the-badge)](./LICENSE)
[![GitHub issues](https://img.shields.io/github/issues/dont-be-evil-company/withsecrets?style=for-the-badge)](https://github.com/dont-be-evil-company/withsecrets/issues)
[![Discord](assets/badge-discord.svg)](https://mistweaverco.com/discord)

[Why?](#why) • [Installation](#installation) • [Usage](#usage)

<p></p>

withsecrets helps you get rid of `.env` files.

Pass env directly from GCP Secret Manager,
AWS Secrets Manager,
Azure Key Vault, OpenBao, and Bitwarden Secrets Manager to your application

<p></p>

</div>

## Table of Contents

- [Why?](#why)
  - [Advantages over other services](#advantages-over-other-services)
- [Installation](#installation)
  - [Manual Installation](#manual-installation)
  - [Automatic Linux and macOS Installation](#automatic-linux-and-macos-installation)
  - [Automatic Windows Installation](#automatic-windows-installation)
- [Usage](#usage)
  - [Configuration File Structure](#configuration-file-structure)
  - [Environment Variable Interpolation](#environment-variable-interpolation)
  - [Secret Path Mapping](#secret-path-mapping)
  - [Running with a specific environment](#running-with-a-specific-environment)
  - [Testing configuration and access](#testing-configuration-and-access)
- [Migrating from kuba](#migrating-from-kuba)
- [Cloud Provider Setup](#cloud-provider-setup)
  - [Google Cloud Platform (GCP)](#google-cloud-platform-gcp)
  - [AWS Secrets Manager](#aws-secrets-manager)
  - [Azure Key Vault](#azure-key-vault)
  - [OpenBao](#openbao)
  - [Bitwarden Secrets Manager](#bitwarden-secrets-manager-bitwarden)

---

## Why?

Environment variables are a common way to manage configuration in applications,
especially when deploying to different environments like development,
staging, and production.

However, managing these variables can become cumbersome,
especially when dealing with multiple cloud providers and
secret management systems.

This often leads to the use of `.env` files,
which can be problematic for several reasons:

- Onboarding new developers, often involves sharing `.env` files.
  This often leads to `.env` files being shared insecurely,
  such as through email or chat applications,
  which can expose sensitive information.
- **Manual Management**: Keeping `.env` files up-to-date with the latest secrets
  from cloud providers can be tedious and error-prone.
- **Security Risks**: `.env` files can accidentally be committed to version control,
  exposing sensitive information.
- **Lack of Standardization**: Each cloud provider has its own way of managing secrets,
  leading to a fragmented approach that can complicate development and deployment.

withsecrets addresses these issues by allowing you to define your environment variables
in a single `ws.yaml` file and fetch them directly from cloud providers like GCP
Secret Manager, AWS Secrets Manager, Azure Key Vault, and OpenBao.

This eliminates the need for `.env` files and provides a more secure,
consistent, and scalable way to manage environment variables across
different environments.

### Advantages over other services

To be clear, there are many other tools that can help you manage secrets:

- [Doppler](https://www.doppler.com/)
- [Vault](https://www.vaultproject.io/)
- [1Password Secrets Automation](https://developer.1password.com/docs/secrets-automation/)
- [Infisical](https://infisical.com/)

... and many more.

> [!CAUTION]
> Most of them require a whopping subscription fee,
> or setting up and maintaining a separate service yourself,
> which can be a barrier for small teams or individual developers.

However, withsecrets is designed to be straightforward and easy to use,
by leveraging the existing secret management systems of cloud providers,
that you might already be using.

## Installation

withsecrets is a single binary, so you can install it easily.

### Manual installation

Download the latest release from [GitHub Releases](https://github.com/dont-be-evil-company/withsecrets/releases/latest).

### Automatic Linux and macOS installation

You can install it using `curl`:

```sh
curl -sSL https://withsecrets.com/install.sh | sh
```

### Automatic Windows installation

Run the following command in PowerShell:

```powershell
iwr https://withsecrets.com/install.ps1 -useb | iex
```

## Migrating from kuba

kuba has been renamed to **withsecrets**. Compatibility with the old command, config file, and paths has been removed.

- The command is `ws`.
- Project config is `ws.yaml` or `withsecrets.yaml`.
- Global config is `~/.config/withsecrets/`.
- `WS_HOME` overrides the app data directory.

Existing `kuba.yaml` files, `KUBA_HOME`, `~/.config/kuba/`, and the `kuba` command are no longer read or installed.

## Usage

```sh
ws run -- <your-application>
```

This will fetch all secrets defined in
`ws.yaml` and pass them as
environment variables to any arbitrary application.

A basic example:

```sh
ws run -- npm run dev
```

### Running commands directly

If you want to run a _command_ with arguments,
use the `--command` flag:

```sh
ws run --command "<your-command> [args...]"
```

When using the `--command` flag,
make sure to wrap the entire command in quotes.

If you don't escape `$` characters,
your shell might try to interpolate them before withsecrets runs.

> [!IMPORTANT]
> Escaping `$` characters is only necessary
> when using the `--command` flag.
>
> When passing an application and its arguments directly,
> withsecrets will handle them correctly.

A basic example with the `--command` flag:

```sh
ws run --command "echo \$DATABASE_URL"
```

> [!NOTE]
> The `--command` flag tries to spawn a shell to run the command,
> so it may behave differently on different platforms.
>
> It tries to use the default shell on your system by
> checking the `$SHELL` environment variable on Unix-like systems

### Debug mode

For troubleshooting configuration issues and seeing detailed execution steps, you can enable debug mode:

```sh
ws --debug run -- <your-application>
# or use the short form
ws -d run -- <your-application>
```

Debug mode provides verbose logging that shows:

- Configuration file discovery and loading
- Environment selection and validation
- Secret provider initialization
- Secret retrieval attempts and results
- Environment variable mapping
- Application execution details

This is particularly useful for:

- Diagnosing cloud provider authentication issues
- Troubleshooting configuration file syntax errors
- Understanding why certain secrets aren't being loaded
- Verifying environment variable interpolation
- Debugging provider-specific errors

### Available commands and flags

withsecrets provides several commands to help you manage your configuration:

- `completion`: Generates shell completion scripts for withsecrets
- `config`: Manages global withsecrets configuration options such as:
  - `cache`: Enable or disable local caching of secrets
  - `defaults`: Manage provider defaults (e.g. default regions)
- `create`: Create withsecrets resources such as templates
  - `template <template_name>`: Create/edit a template in the user templates directory
- `convert`: Converts existing configuration sources (e.g. **dotenv** (`.env*`), **Knative Service** manifests) to `ws.yaml` format
- `changelog [latest|version]`: Shows the contents of `CHANGELOG.md` (during build-time) in the terminal
- `help`: Displays help information for withsecrets and its commands
- `init [template]`: Initializes a new `ws.yaml` using a template
- `run`: Runs an application with environment variables fetched from secrets
- `show`: Displays the effective environment variables for a given configuration
- `test`: Tests the configuration and secret retrieval without running an application
- `update`: Updates withsecrets to the latest version
- `version`: Displays the current version of withsecrets

```sh
# Initialize a new configuration file
ws init

# Initialize using a named template from ~/.config/withsecrets/templates
ws init my-template

# Create or edit a template in ~/.config/withsecrets/templates
ws create template my-template

# Run a command with secrets
ws run -- <application> [args...]

# Test secret retrieval without running a command
ws test --env <environment>

# Show version information
ws version

# Show changelog (contents at built-time)
ws changelog

# Show latest release notes only
ws changelog latest

# Show a specific version section only
ws changelog v1.7.0

# Get help
ws --help
```

**Global Flags:**

- `--debug, -d`: Enable debug mode for verbose logging
- `--version`: Show version information
- `--help, -h`: Show help information

**Run Command Flags:**

- `--env, -e`: Specify environment (default: "default")
- `--config, -c`: Path to configuration file
- `--contain`: Only use environment variables from ws.yaml, do not merge with OS environment

**Test Command Flags:**

- `--env, -e`: Specify environment (default: "default")
- `--config, -c`: Path to configuration file

Let's say you want to pass
some secrets from GCP to your node application.

```sh
ws run -- node dist/server.js
```

### Using the `--contain` flag

The `--contain` flag prevents the merging of the current OS environment with
the environment variables from `ws.yaml`.

This is useful when you want to ensure only the secrets defined in
your configuration are available to the application.

```sh
# Only use environment variables from ws.yaml
ws run --contain -- node dist/server.js

# Useful for Docker containers to avoid inheriting host environment
docker run --env-file=<(ws run --contain -- env) your-container
```

and your `ws.yaml` would look something like this:

```yaml
# yaml-language-server: $schema=https://withsecrets.com/ws.schema.json
---
# Top-level sections for different environments.
default:
  provider: gcp
  project: 1337

  # Mapping of cloud projects to environment variables and secret keys.
  env:
    GCP_PROJECT_ID:
      secret-key: "gcp_project_secret"
    AWS_PROJECT_ID:
      secret-key: "aws_project_secret"
      provider: aws
    AZURE_PROJECT_ID:
      secret-key: "azure_project_secret"
      provider: azure
      project: "my-azure-project-default"
    OPENBAO_SECRET:
      secret-key: "secret/openbao-secret"
      provider: openbao
    DATABASE_CONFIG:
      secret-path: "database"
    API_KEYS:
      secret-path: "external-apis"
      provider: aws
    SOME_HARD_CODED_ENV:
      value: "hard-coded-value"

---
# Settings for the development environment.
development:
  provider: gcp
  project: 1337

  # You can override specific mappings here or add new ones.
  env:
    DEV_GCP_PROJECT_ID:
      secret-key: "dev_gcp_project_secret"
    DEV_AWS_PROJECT_ID:
      secret-key: "dev_aws_project_secret"
      provider: aws

---
# Settings for the staging environment.
staging:
  provider: gcp
  project: 1337

  env:
    STAGING_GCP_PROJECT_ID:
      secret-key: "staging_gcp_project_secret"
    STAGING_AWS_PROJECT_ID:
      secret-key: "staging_aws_project_secret"
      provider: aws
---
# Settings for the production environment.
production:
  provider: gcp
  project: 1337

  env:
    PROD_GCP_PROJECT_ID:
      secret-key: "prod_gcp_project_secret"
    PROD_AWS_PROJECT_ID:
      secret-key: "prod_aws_project_secret"
      provider: aws
```

This `ws.yaml` file defines the secrets for different environments
and maps them to environment variables. The example includes:

- **Individual secrets** using `secret-key`
  (e.g., GCP_PROJECT_ID, AWS_PROJECT_ID)
- **Secret paths** using `secret-path` to
  fetch all secrets under a prefix (e.g., DATABASE_CONFIG, API_KEYS)
- **Hard-coded values** using `value` for static configuration
- **Cross-provider mappings** where different secrets come
  from different cloud providers

### Confguration file structure

Each top-level section corresponds to a different environment,
such as `default`, `development`, `staging`, and `production`.

They're completely arbitrary.

The exception is `default`,
which is the default to use, when no other `--env` is specified

Each section specifies the cloud provider, the project ID,
and a list of mappings between environment variables and secret keys.

You can also specify the provider and project ID for each mapping,
allowing you to fetch secrets from different cloud providers
or projects as needed. withsecrets currently supports GCP Secret Manager,
AWS Secrets Manager, Azure Key Vault, and OpenBao.

### Environment variable interpolation

withsecrets supports environment variable interpolation
in the `value` field using `${VAR_NAME}` syntax.

This allows you to:

- Reference previously defined environment variables from the same configuration
- Use system environment variables
- Build complex connection strings and URLs dynamically

**Example with interpolation:**

```yaml
default:
  provider: gcp
  project: 1337
  env:
    DB_PASSWORD:
      secret-key: "db-password"
    DB_HOST:
      value: "mydbhost"
    DB_CONNECTION_STRING:
      value: "postgresql://user:${DB_PASSWORD}@${DB_HOST}:5432/mydb"
    API_URL:
      value: "https://api.${DOMAIN}/v1"
    APP_ENV:
      value: "${NODE_ENV:-development}"
    REDIS_URL:
      value: "redis://${REDIS_HOST:-localhost}:${REDIS_PORT:-6379}/0"
```

In this example:

- `${DB_PASSWORD}` will be replaced with the value from the secret
- `${DB_HOST}` will be replaced with the literal value "mydbhost"
- `${DOMAIN}` will be replaced with the system environment variable if it exists
- `${NODE_ENV:-development}` will use the `NODE_ENV` environment variable if set, otherwise default to "development"
- `${REDIS_HOST:-localhost}` will use the `REDIS_HOST` environment variable if set, otherwise default to "localhost"

**Note**: Interpolation is processed in order,
so you can reference variables defined earlier in the same configuration.
Unresolved variables will remain unchanged in the output.

**Shell-style default values**: You can use `${VAR_NAME:-default}` syntax to
provide fallback values when environment variables aren't set.

This is particularly useful for providing sensible defaults
while allowing overrides through environment variables.

**Environment variable naming**:
All environment variable names (including those from secrets) are
automatically sanitized to be valid POSIX environment variable names.

This means:

- Names are converted to uppercase
- Non-alphanumeric characters are replaced with underscores
- Names that don't start with a letter or underscore get a leading underscore
- This ensures compatibility across different operating systems and shells

### Secret path mapping

In addition to individual secret keys, withsecrets supports **secret path mapping** using the `secret-path` field.
This feature allows you to fetch all secrets that start with a given path prefix,
which is particularly useful for:

- **Bulk secret retrieval**: Fetch all secrets under a specific namespace or directory
- **Organized secret management**: Group related secrets under common path prefixes
- **Environment-specific configurations**: Load all secrets for a specific environment or service

**How it works:**

- When you specify a `secret-path`, withsecrets will fetch all secrets that start with that path
- `secret-path` and `param-path` may be a string or an ordered list of strings; later paths overlay earlier ones when names collide
- Each secret found will be converted to an environment variable using the pattern: `{ENVIRONMENT_VARIABLE}_{RELATIVE_NAME}` (relative name = sanitized suffix after the path)
- Use env key `"*"` with `secret-path` or `param-path` to inject relative names **without** a prefix
- Secret names are automatically sanitized to be valid POSIX environment variable names (uppercase, underscores only)

**Example with secret paths:**

```yaml
default:
  provider: gcp
  project: 1337
  env:
    DB:
      secret-path: "database"
    API:
      secret-path: "external-apis"
    SERVICE:
      secret-path: "microservices"
    "*":
      secret-path:
        - /shared/config
        - /overrides/config   # later wins on name collisions
    HARD_CODED:
      value: "static-value"
```

If your GCP Secret Manager contains secrets like:

- `database-connection-string`
- `database-username`
- `database-password`
- `external-apis-stripe-key`
- `external-apis-sendgrid-key`
- `microservices-auth-service-token`
- `microservices-user-service-token`

withsecrets will create these environment variables:

- `DB_CONNECTION_STRING` = value of `database-connection-string`
- `DB_USERNAME` = value of `database-username`
- `DB_PASSWORD` = value of `database-password`
- `API_STRIPE_KEY` = value of `external-apis-stripe-key`
- `API_SENDGRID_KEY` = value of `external-apis-sendgrid-key`
- `SERVICE_AUTH_SERVICE_TOKEN` = value of `microservices-auth-service-token`
- `SERVICE_USER_SERVICE_TOKEN` = value of `microservices-user-service-token`

**Cross-provider secret paths:**
You can also use secret paths with different providers:

```yaml
default:
  provider: gcp
  project: 1337
  env:
    GCP_SECRETS:
      secret-path: "app-config"
      provider: gcp
    AWS_SECRETS:
      secret-path: "app-config"
      provider: aws
    AZURE_SECRETS:
      secret-path: "app-config"
      provider: azure
      project: "my-azure-project"
```

**Important notes:**

- Secret paths work with all supported providers (GCP, AWS, Azure, OpenBao)
- The resulting environment variable names are automatically sanitized and uppercased
- You can mix `secret-key`, `secret-path`, and `value` mappings in the same configuration
- Secret paths are processed after individual secret keys, so you can reference path-based variables in value interpolations

### Running with a specific environment

You can also specify the environment you want to use:

```sh
ws run --env development -- node dist/server.js
```

### Testing configuration and access

Use the `test` subcommand to verify that withsecrets can load your configuration and
retrieve all mapped values for an environment without executing any application:

```sh
# Use default environment
ws test

# Specify an environment
ws test --env staging

# Point to a specific configuration file
ws test --config ./config/ws.yaml --env production
```

This is useful for validating credentials, permissions, and
configuration mappings during setup or CI.

### Update ws to the latest version

To update ws to the latest version, run:

```sh
ws update
```

This fetches the latest release and replaces your
existing ws binary.

It also backups your current binary in case you need to revert.

### Converting dotenv (`.env*`) files

To convert existing dotenv files to `ws.yaml` format,

```sh
# Convert .env to ws.yaml for the default environment
ws convert --infile .env

# Convert .env.staging to my-ws.yaml for the staging environment
ws convert --infile .env.staging --outfile my-ws.yaml --env staging
```

### Converting Knative Service (`ksvc`) manifests

If you already have a Knative Service (for example a Cloud Run service) with all
your environment variables defined, you can generate a `ws.yaml` from that
manifest:

```sh
# Convert a Knative Service manifest to ws.yaml for the production environment
ws convert --from ksvc --infile service.yaml --env production
```

withsecrets will:

- Read the container `env` section of the Service template.
- Convert hard-coded `value` entries into `value` mappings in `ws.yaml`.
- Convert `valueFrom.secretKeyRef` entries into `secret-key` mappings, using the
  Kubernetes Secret name as the secret identifier.

For Knative Services running on GCP (Cloud Run), the generated environment will
default to:

- `provider: gcp`
- `project`: the Service `metadata.namespace` (typically the GCP project number)

### Show effective environment variables

You can use the `show` subcommand to display the
effective environment variables for a given configuration:

```sh
# Show environment variables for the default environment
ws show

# Show environment variables for the production environment
ws show --env production

# Point to a specific configuration file and environment
ws show --config ./config/ws.yaml --env staging

# Redact sensitive values
ws show --sensitive

# Filter by one or more prefixes (case-insensitive)
ws show "db*" "api*"
```

## Cloud Provider Setup

The following providers are supported:

- GCP Secret Manager (`gcp`)
- AWS Secrets Manager (`aws`)
- Azure Key Vault (`azure`)
- OpenBao (`openbao`)
- Bitwarden Secrets Manager (`bitwarden`)
- Local (`local`, use for hard-coded values only)

### Bitwarden Secrets Manager (bitwarden)

withsecrets supports Bitwarden Secrets Manager via the official Bitwarden Go SDK. To use Bitwarden:

1. **Authentication & organization**

   Configure an access token and organization ID:

   ```sh
   export BITWARDEN_ACCESS_TOKEN="your-access-token" # or ACCESS_TOKEN
   export BITWARDEN_ORGANIZATION_ID="your-organization-id"
   ```

   You can also set the organization ID in your `ws.yaml` via the `project` field when using the
   `bitwarden` provider; that value is treated as the Bitwarden organization ID.

2. **Self-hosted Bitwarden (optional)**

   To use a self-hosted Bitwarden instance, configure:

   ```sh
   export BITWARDEN_API_URL="https://your-bitwarden.example.com/api"
   export BITWARDEN_IDENTITY_URL="https://your-bitwarden.example.com/identity"
   ```

3. **Persisting Bitwarden state (optional)**

   By default, withsecrets authenticates the Bitwarden SDK on each run using your access token only. If you
   want the SDK to reuse its own state between runs (for example to avoid re-initializing some
   internal session data), you can point it at a state file:

   ```sh
   export BITWARDEN_STATE_FILE="$HOME/.local/share/withsecrets/bitwarden_state.json"
   ```

   When this variable is set:

   - The Bitwarden SDK will create or update the file itself when withsecrets calls `AccessTokenLogin`.
   - Subsequent withsecrets runs will pass the same state file to the SDK so it can reuse whatever it
     stored there.

   **Recommendations:**

   - Treat the state file as sensitive: keep it outside your repo and out of version control.
   - Prefer a user-scoped **data** directory such as:
     - Linux/macOS: `~/.local/share/withsecrets/bitwarden_state.json`
     - Windows (PowerShell): `"$Env:LOCALAPPDATAwithsecretsbitwarden_state.json"`
   - You still need a valid Bitwarden access token; the state file complements it rather than
     replacing it.

4. **Configuration**

   In your `ws.yaml`, specify the Bitwarden provider. The `secret-key` entries must refer to
   Bitwarden **secret IDs**:

   ```yaml
   default:
     provider: bitwarden
     # Optional: if omitted, BITWARDEN_ORGANIZATION_ID must be set
     project: "your-bitwarden-organization-id"
     env:
       DATABASE_URL:
         secret-key: "bitwarden-secret-id-for-database-url"
       API_KEY:
         secret-key: "bitwarden-secret-id-for-api-key"
       SOME_HARD_CODED_ENV:
         value: "hard-coded-value"
   ```

   > **Note**
   >
   > Bitwarden does not currently support hierarchical secret-path lookups in withsecrets; only
   > `secret-key` mappings (by secret ID) are supported for the `bitwarden` provider.

### Google Cloud Platform (gcp)

withsecrets supports GCP Secret Manager for fetching secrets. To use GCP:

1. **Enable Secret Manager API**: Make sure the Secret Manager API is enabled in your GCP project.

2. **Authentication**: Set up authentication using one of these methods:
   - **Service Account Key**: Set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable to point to your service account JSON key file:
     ```sh
     export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"
     ```
   - **Application Default Credentials**: Use `gcloud auth application-default login` to set up local development credentials
   - **Workload Identity**: If running on GKE or other GCP services, use workload identity
   - **Compute Engine**: If running on Compute Engine, the default service account will be used automatically

3. **IAM Permissions**: Ensure your service account has the `Secret Manager Secret Accessor` role for the secrets you want to access.

4. **Example Configuration**:

   ```yaml
   default:
     provider: gcp
     project: 1337
     env:
       DATABASE_URL:
         secret-key: "database-connection-string"
       API_KEY:
         secret-key: "external-api-key"
       SOME_HARD_CODED_ENV:
         value: "hard-coded-value"
   ```

### AWS Secrets Manager & Parameter Store (aws)

withsecrets supports AWS Secrets Manager and Systems Manager Parameter Store. To use AWS:

1. **Authentication**: Set up authentication using one of these methods:
   - **Environment Variables**: Set `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`:
     ```sh
     export AWS_ACCESS_KEY_ID="your-access-key"
     export AWS_SECRET_ACCESS_KEY="your-secret-key"
     export AWS_REGION="us-east-1"
     ```
   - **AWS Profile**: Set `AWS_PROFILE` to use a specific profile from your AWS credentials file:
     ```sh
     export AWS_PROFILE="my-profile"
     export AWS_REGION="us-east-1"
     ```
   - **IAM Roles**: If running on EC2, ECS, or other AWS services, use IAM roles
   - **AWS CLI**: Use `aws configure` to set up your credentials

2. **IAM Permissions**:
   - Secrets Manager: `secretsmanager:GetSecretValue` (and `ListSecrets` for `secret-path`)
   - Parameter Store: `ssm:GetParameter` / `ssm:GetParametersByPath` (plus `kms:Decrypt` for SecureString)

3. **Region**: Set optional `region` in `ws.yaml` (environment or per mapping). If unset, falls back to `AWS_REGION`, then `AWS_DEFAULT_REGION`, then the shared AWS config.

4. **Example Configuration (Secrets Manager)**:

   ```yaml
   default:
     provider: aws
     region: eu-west-3
     env:
       DATABASE_URL:
         secret-key: "database-connection-string"
       API_KEY:
         secret-key: "external-api-key"
       SOME_HARD_CODED_ENV:
         value: "hard-coded-value"
   ```

5. **Parameter Store** uses `param-key` / `param-path`. With env key `"*"`, path suffixes become env vars with no prefix:

   ```yaml
   default:
     provider: aws
     region: eu-west-3
     env:
       "*":
         param-path:
           - /local/withsecrets/config
           - /local/withsecrets/overrides   # later wins on name collisions
   ```

   Parameters `/local/withsecrets/config/USERNAME` and `/local/withsecrets/config/PASSWORD` inject as `USERNAME` and `PASSWORD`. The same `"*"` pattern works with `secret-path` on all providers.

### Azure Key Vault (azure)

withsecrets supports Azure Key Vault for fetching secrets. To use Azure Key Vault:

1. **Authentication**: withsecrets supports multiple authentication methods:
   - **Service Principal**: Set the following environment variables:
     ```bash
     export AZURE_KEY_VAULT_URL="https://yourvault.vault.azure.net/"
     export AZURE_TENANT_ID="your-tenant-id"
     export AZURE_CLIENT_ID="your-client-id"
     export AZURE_CLIENT_SECRET="your-client-secret"
     ```
   - **Managed Identity**: If running on Azure services with managed identity enabled
   - **Default Azure Credential**: Uses Azure CLI, Visual Studio Code, or other Azure tools

2. **Key Vault Permissions**: Ensure your Azure credentials have the `Get` and `List` permissions for secrets in your Key Vault.

3. **Configuration**: In your `ws.yaml`, specify the Azure provider:
   ```yaml
   default:
     provider: azure
     env:
       DATABASE_URL:
         secret-key: "database-connection-string"
       SOME_HARD_CODED_ENV:
         value: "hard-coded-value"
   ```

### OpenBao (openbao)

withsecrets supports OpenBao for fetching secrets.
OpenBao is a fork of HashiCorp Vault that provides secure secret storage and access.

To use OpenBao:

1. **Setup**: Make sure you have an OpenBao server running and accessible.

2. **Authentication**: Set up authentication using environment variables:

   ```bash
   export OPENBAO_ADDR="http://localhost:8200"  # Required: OpenBao server address
   export OPENBAO_TOKEN="your-openbao-token"    # Optional: Authentication token
   export OPENBAO_NAMESPACE="your-namespace"     # Optional: Namespace (if using enterprise features)
   ```

3. **Permissions**: Ensure your OpenBao token has read permissions for the secrets you want to access.

4. **Configuration**: In your `ws.yaml`, specify the OpenBao provider:
   ```yaml
   default:
     provider: openbao
     env:
       DATABASE_URL:
         secret-key: "secret/database-url"
       API_KEY:
         secret-key: "secret/api-key"
       SOME_HARD_CODED_ENV:
         value: "hard-coded-value"
   ```

**Note**: `project` is a path prefix and `secret-key` is the rest of the secret path. For a KV v2 mount, withsecrets inserts the `/data/` API segment after the mount (so `project: secretv2` and `secret-key: withsecrets/foo/bar` read `secretv2/data/withsecrets/foo/bar`). A path that already contains `/data/` is left as-is. The KV v2 response is unwrapped from `data.data`.

If a secret contains one string, that value is returned. If it contains several, set `field` to choose one key (the same as `bao kv get -field`). Without `field`, withsecrets returns the first string value it finds.

```yaml
default:
  provider: openbao
  project: secretv2
  env:
    FOOBAR:
      secret-key: "withsecrets/foo/bar"
      field: "password"
```
