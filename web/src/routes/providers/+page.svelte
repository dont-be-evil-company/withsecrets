<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
	import CodeBlock from '$lib/CodeBlock.svelte';
</script>

<HeadComponent
	data={{
		title: 'Providers Setup - withsecrets',
		description:
			'Set up authentication and permissions for GCP, AWS, Azure, OpenBao, Bitwarden, and local providers to use with withsecrets.'
	}}
/>

<div class="container mx-auto px-4 py-8">
	<div class="max-w-4xl mx-auto">
		<div class="text-center mb-12">
			<ClickableHeadline level={1} id="providers-setup" className="text-4xl font-bold mb-4"
				>Providers Setup</ClickableHeadline
			>
			<p class="text-xl text-base-content/70">
				Configure authentication and permissions for your providers to start using withsecrets
				securely.
			</p>
		</div>

		<div class="space-y-12">
			<section>
				<ClickableHeadline level={2} id="supported-providers" className="text-3xl font-bold mb-6"
					>Supported Providers</ClickableHeadline
				>

				<div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">☁️</div>
							<a class="hover:link" href="#gcp">
								<h3 class="card-title justify-center">Google Cloud Platform</h3>
							</a>
							<p class="text-sm">
								Secret Manager integration with service accounts and workload identity
							</p>
						</div>
					</div>

					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">☁️</div>
							<a class="hover:link" href="#aws">
								<h3 class="card-title justify-center">AWS</h3>
							</a>
							<p class="text-sm">Secrets Manager with IAM roles and access keys</p>
						</div>
					</div>

					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">☁️</div>
							<a class="hover:link" href="#azure">
								<h3 class="card-title justify-center">Azure</h3>
							</a>
							<p class="text-sm">Key Vault with service principals and managed identity</p>
						</div>
					</div>

					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">☁️</div>
							<a class="hover:link" href="#openbao">
								<h3 class="card-title justify-center">OpenBao</h3>
							</a>
							<p class="text-sm">Self-hosted secrets with tokens and namespaces</p>
						</div>
					</div>

					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">🔐</div>
							<a class="hover:link" href="#bitwarden">
								<h3 class="card-title justify-center">Bitwarden Secrets Manager</h3>
							</a>
							<p class="text-sm">Bitwarden Secrets Manager with cloud or self-hosted instances</p>
						</div>
					</div>

					<div class="card bg-base-200 text-center">
						<div class="card-body">
							<div class="text-4xl mb-2">🛠️</div>
							<a class="hover:link" href="#local">
								<h3 class="card-title justify-center">Local</h3>
							</a>
							<p class="text-sm">Use for hard-coded non-sensitive values during development</p>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="bitwarden" className="text-3xl font-bold mb-6"
					>Bitwarden Secrets Manager (bitwarden)</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="bitwarden-authentication" className="card-title"
								>1. Authentication & Organization</ClickableHeadline
							>
							<p class="mb-4">
								Configure a Bitwarden Secrets Manager access token and organization ID:
							</p>
							<CodeBlock
								lang="bash"
								code={`export BITWARDEN_ACCESS_TOKEN="your-access-token"    # or ACCESS_TOKEN
export BITWARDEN_ORGANIZATION_ID="your-organization-id"`}
							/>
							<p class="mt-4 text-sm">
								You can also set the organization ID in your <code>ws.yaml</code> via the
								<code>project</code> field when using the <code>bitwarden</code> provider; that value
								is treated as the Bitwarden organization ID.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="bitwarden-self-hosted" className="card-title"
								>2. Self-hosted Bitwarden (optional)</ClickableHeadline
							>
							<p class="mb-4">
								To use a self-hosted Bitwarden instance, configure the API and identity URLs:
							</p>
							<CodeBlock
								lang="bash"
								code={`export BITWARDEN_API_URL="https://your-bitwarden.example.com/api"
export BITWARDEN_IDENTITY_URL="https://your-bitwarden.example.com/identity"`}
							/>
							<p class="mt-4 text-sm">
								Optional: if you want the Bitwarden SDK to reuse its own state between withsecrets
								runs (for example, to avoid re-initializing some internal session data), you can
								point it at a state file. The SDK will create and maintain this file itself:
							</p>
							<CodeBlock
								lang="bash"
								code={`export BITWARDEN_STATE_FILE="$HOME/.local/share/withsecrets/bitwarden_state.json"`}
							/>
							<p class="mt-2 text-sm">
								Treat this file as sensitive: keep it outside version control and in a user-scoped
								data directory (for example, <code>~/.local/share/withsecrets</code> on Linux/macOS
								or
								<code>%LOCALAPPDATA%\withsecrets</code> on Windows). You still need a valid Bitwarden
								access token; the state file complements it rather than replacing it.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">3. Configuration Example</h3>
							<p class="mb-4">
								In your <code>ws.yaml</code>, use the <code>bitwarden</code> provider. The
								<code>secret-key</code> values must be Bitwarden <strong>secret IDs</strong>:
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: bitwarden
  # Optional: if omitted, BITWARDEN_ORGANIZATION_ID must be set
  project: "your-bitwarden-organization-id"
  env:
    DATABASE_URL:
      secret-key: "bitwarden-secret-id-for-database-url"
    API_KEY:
      secret-key: "bitwarden-secret-id-for-api-key"
    SOME_HARD_CODED_ENV:
      value: "hard-coded-value"`}
							/>
							<p class="mt-4 text-sm">
								<strong>Note:</strong> Bitwarden support in withsecrets currently only supports
								<code>secret-key</code> mappings (by secret ID). <code>secret-path</code> mappings
								are not available for the <code>bitwarden</code> provider.
							</p>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="gcp" className="text-3xl font-bold mb-6"
					>Google Cloud Platform (gcp)</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="gcp-authentication-methods" className="card-title"
								>1. Authentication Methods</ClickableHeadline
							>
							<div class="space-y-4">
								<div>
									<h4 class="font-bold">Application Default Credentials</h4>
									<p class="mb-2">Use gcloud for local development:</p>
									<CodeBlock lang="bash" code={`gcloud auth application-default login`} />
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">Service Account Key</h4>
									<p class="mb-2">
										Set the <code>GOOGLE_APPLICATION_CREDENTIALS</code> environment variable:
									</p>
									<CodeBlock
										lang="bash"
										code={`export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"`}
									/>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">Workload Identity</h4>
									<p class="mb-2">For GKE or other GCP services, use workload identity.</p>
								</div>
								<div class="alert alert-info">
									<i class="fa-solid fa-info-circle"></i>
									<p>
										If running on Compute Engine, the default service account will be used
										automatically.
									</p>
									<div></div>
								</div>
							</div>
						</div>

						<div class="card bg-base-200">
							<div class="card-body">
								<h3 class="card-title">2. Enable Secret Manager API</h3>
								<p class="mb-4">Make sure the Secret Manager API is enabled in your GCP project:</p>
								<CodeBlock
									lang="bash"
									code={`gcloud services enable secretmanager.googleapis.com`}
								/>
							</div>
						</div>

						<div class="card bg-base-200">
							<div class="card-body">
								<h3 class="card-title">3. IAM Permissions</h3>
								<p class="mb-4">
									Ensure your service account has the <code>Secret Manager Secret Accessor</code> role:
								</p>
								<CodeBlock
									lang="bash"
									code={`gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:YOUR_SERVICE_ACCOUNT@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"`}
								/>
							</div>
						</div>
						<div class="card bg-base-200">
							<div class="card-body">
								<h3 class="card-title">4. Configuration Example</h3>
								<CodeBlock
									lang="yaml"
									meta="path=ws.yaml"
									code={`default:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "database-connection-string"
    API_KEY:
      secret-key: "external-api-key"`}
								/>
							</div>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="aws" className="text-3xl font-bold mb-6"
					>AWS Secrets Manager &amp; Parameter Store (aws)</ClickableHeadline
				>
				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="aws-authentication-methods" className="card-title"
								>1. Authentication Methods</ClickableHeadline
							>
							<div class="space-y-4">
								<div>
									<h4 class="font-bold">Environment Variables</h4>
									<p class="mb-2">Set AWS credentials:</p>
									<CodeBlock
										lang="bash"
										code={`export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"`}
									/>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">AWS Profile</h4>
									<p class="mb-2">Use a specific profile from your AWS credentials file:</p>
									<CodeBlock
										lang="bash"
										code={`export AWS_PROFILE="my-profile"
export AWS_REGION="us-east-1"`}
									/>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">IAM Roles</h4>
									<p class="mb-2">If running on EC2, ECS, or other AWS services, use IAM roles.</p>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">AWS CLI</h4>
									<p class="mb-2">Use <code>aws configure</code> to set up your credentials.</p>
								</div>
							</div>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">2. IAM Permissions</h3>
							<p class="mb-4">
								For <strong>Secrets Manager</strong>, grant
								<code>secretsmanager:GetSecretValue</code>
								(and <code>secretsmanager:ListSecrets</code> for <code>secret-path</code> bulk loading):
							</p>
							<CodeBlock
								lang="json"
								code={`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:ListSecrets"
      ],
      "Resource": "*"
    }
  ]
}`}
							/>
							<p class="mb-4 mt-4">
								For <strong>Parameter Store</strong>, grant <code>ssm:GetParameter</code> /
								<code>ssm:GetParametersByPath</code> (and decryption if you use SecureString):
							</p>
							<CodeBlock
								lang="json"
								code={`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:DescribeParameters"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": ["kms:Decrypt"],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "ssm.*.amazonaws.com"
        }
      }
    }
  ]
}`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">3. Configuration Example (Secrets Manager)</h3>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: aws
  region: eu-west-3
  env:
    DATABASE_URL:
      secret-key: "database-connection-string"
    API_KEY:
      secret-key: "external-api-key"`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">4. Parameter Store (param-key / param-path)</h3>
							<p class="mb-4">
								Use <code>param-key</code> for a single parameter, or <code>param-path</code> to
								load everything under a path. With env key <code>"*"</code>, each parameter name
								suffix is injected as its own environment variable (no prefix):
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: aws
  region: eu-west-3
  env:
    USERNAME:
      param-key: /local/withsecrets/config/USERNAME
    "*":
      param-path: /local/withsecrets/config`}
							/>
							<p class="mt-4 text-sm">
								If <code>/local/withsecrets/config/USERNAME</code> and
								<code>/local/withsecrets/config/PASSWORD</code> exist, the <code>"*"</code> mapping
								injects <code>USERNAME</code> and <code>PASSWORD</code>. Optional
								<code>region</code> overrides <code>AWS_REGION</code> /
								<code>AWS_DEFAULT_REGION</code>.
							</p>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="azure" className="text-3xl font-bold mb-6"
					>Azure Key Vault (azure)</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="azure-authentication-methods" className="card-title"
								>1. Authentication Methods</ClickableHeadline
							>
							<div class="space-y-4">
								<div>
									<h4 class="font-bold">Service Principal</h4>
									<p class="mb-2">Set the following environment variables:</p>
									<CodeBlock
										lang="bash"
										code={`export AZURE_KEY_VAULT_URL="https://yourvault.vault.azure.net/"
export AZURE_TENANT_ID="your-tenant-id"
export AZURE_CLIENT_ID="your-client-id"
export AZURE_CLIENT_SECRET="your-client-secret"`}
									/>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">Managed Identity</h4>
									<p class="mb-2">If running on Azure services with managed identity enabled.</p>
								</div>
								<div class="divider">OR</div>
								<div>
									<h4 class="font-bold">Default Azure Credential</h4>
									<p class="mb-2">Uses Azure CLI, Visual Studio Code, or other Azure tools.</p>
								</div>
							</div>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">2. Key Vault Permissions</h3>
							<p class="mb-4">
								Ensure your Azure credentials have the <code>Get</code> and <code>List</code> permissions
								for secrets in your Key Vault.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">3. Configuration Example</h3>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: azure
  env:
    DATABASE_URL:
      secret-key: "database-connection-string"
    API_KEY:
      secret-key: "external-api-key"`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="openbao" className="text-3xl font-bold mb-6"
					>OpenBao (openbao)</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">1. Setup</h3>
							<p class="mb-4">Make sure you have an OpenBao server running and accessible.</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="openbao-authentication-methods"
								className="card-title">2. Authentication Methods</ClickableHeadline
							>
							<p class="mb-4">Set up authentication using environment variables:</p>
							<CodeBlock
								lang="bash"
								code={`export OPENBAO_ADDR="http://localhost:8200"  # Required: OpenBao server address
export OPENBAO_TOKEN="your-openbao-token"    # Optional: Authentication token
export OPENBAO_NAMESPACE="your-namespace"     # Optional: Namespace (if using enterprise features)`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">3. Permissions</h3>
							<p class="mb-4">
								Ensure your OpenBao token has read permissions for the secrets you want to access.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">4. Configuration Example</h3>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: openbao
  env:
    DATABASE_URL:
      secret-key: "secret/database-url"
    API_KEY:
      secret-key: "secret/api-key"`}
							/>
							<p class="mt-4 text-sm">
								<code>project</code> is the mount or path prefix. For a KV v2 mount, withsecrets
								inserts
								<code>/data/</code> after the mount, then unwraps <code>data.data</code>. Use
								<code>field</code> to select one key, the same as <code>bao kv get -field</code>.
								<code>field</code> is only valid for the openbao provider, together with
								<code>secret-key</code>. Without <code>field</code>, a multi-key secret returns the
								first string value. A path that already includes <code>/data/</code> is left unchanged.
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: openbao
  project: secretv2
  env:
    FOOBAR:
      secret-key: "withsecrets/foo/bar"
      field: "password"`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="local" className="text-3xl font-bold mb-6"
					>Local (local)</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="alert alert-error text-xl">
						<i class="fa-solid fa-triangle-exclamation"></i>
						<span>
							Make sure to use the <code>local</code> provider only for non-sensitive values.
						</span>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">2. Configuration Example</h3>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: local
  env:
    LOG_ENV:
      value: "local"
    LOG_LEVEL:
      value: "debug"`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<h2 class="text-3xl font-bold mb-6">Multi-Provider Configuration</h2>

				<div class="card bg-base-200">
					<div class="card-body">
						<h3 class="card-title">Using Multiple Providers</h3>
						<p class="mb-4">You can use different cloud providers in the same configuration:</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`default:
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
    OPENBAO_SECRETS:
      secret-path: "app-config"
      provider: openbao
    BITWARDEN_DATABASE:
      # Bitwarden secrets are addressed by secret ID; this maps a single secret ID
      secret-key: "bitwarden-secret-id-for-database-url"
      provider: bitwarden`}
						/>
					</div>
				</div>
			</section>

			<section>
				<h2 class="text-3xl font-bold mb-6">Security Best Practices</h2>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Authentication</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Use service accounts with minimal permissions</li>
								<li>Rotate credentials regularly</li>
								<li>Use managed identities when possible</li>
								<li>Avoid hardcoding credentials</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Permissions</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Follow principle of least privilege</li>
								<li>Use role-based access control</li>
								<li>Limit access to production secrets</li>
								<li>Monitor access patterns</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Network Security</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Use private networks when possible</li>
								<li>Enable VPC endpoints for AWS</li>
								<li>Use private service connect for GCP</li>
								<li>Restrict access by IP when applicable</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Monitoring</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Enable audit logging</li>
								<li>Set up alerts for unusual access</li>
								<li>Monitor secret rotation</li>
								<li>Track usage patterns</li>
							</ul>
						</div>
					</div>
				</div>
			</section>

			<section>
				<h2 class="text-3xl font-bold mb-6">Troubleshooting</h2>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Common Issues</h3>
							<div class="space-y-4">
								<div>
									<h4 class="font-bold">Authentication Errors</h4>
									<p class="text-sm">
										Check your credentials and ensure they haven't expired. Verify the
										authentication method you're using.
									</p>
								</div>
								<div>
									<h4 class="font-bold">Permission Errors</h4>
									<p class="text-sm">
										Ensure your credentials have the necessary permissions to access the secrets
										specified in your configuration.
									</p>
								</div>
								<div>
									<h4 class="font-bold">Network Issues</h4>
									<p class="text-sm">
										Check your network connectivity and firewall settings. Ensure you can reach the
										cloud provider APIs.
									</p>
								</div>
							</div>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Debug Mode</h3>
							<p class="mb-4">
								Enable debug mode to see detailed information about authentication and API calls:
							</p>
							<CodeBlock lang="bash" code={`ws run --debug -- echo "Testing connection"`} />
						</div>
					</div>
				</div>
			</section>

			<section>
				<h2 class="text-3xl font-bold mb-6">Next Steps</h2>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Configuration Guide</h3>
							<p>Learn how to set up your <code>ws.yaml</code> configuration file.</p>
							<a href="/configuration" class="btn btn-outline bg-lg">Configuration Guide</a>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Usage Examples</h3>
							<p>See practical examples of how to use your configured providers.</p>
							<a href="/examples" class="btn btn-outline bg-lg">Examples Guide</a>
						</div>
					</div>
				</div>
			</section>
		</div>
	</div>
</div>
