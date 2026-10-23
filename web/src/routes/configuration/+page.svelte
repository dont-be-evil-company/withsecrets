<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
	import CodeBlock from '$lib/CodeBlock.svelte';
</script>

<HeadComponent
	data={{
		title: 'Configuration Guide - withsecrets',
		description:
			'Learn how to configure withsecrets with ws.yaml, environment variable interpolation, and secret path mapping.'
	}}
/>

<div class="container mx-auto px-4 py-8">
	<div class="max-w-4xl mx-auto">
		<div class="text-center mb-12">
			<ClickableHeadline level={1} id="configuration-guide" className="text-4xl font-bold mb-4"
				>Configuration Guide</ClickableHeadline
			>
			<p class="text-xl text-base-content/70">
				Learn how to configure withsecrets with the <code>ws.yaml</code> file and understand advanced
				features like variable interpolation and secret paths.
			</p>
		</div>

		<div class="space-y-12">
			<section>
				<ClickableHeadline level={2} id="getting-started" className="text-3xl font-bold mb-6"
					>Getting Started</ClickableHeadline
				>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<h3 class="card-title">Initialize Configuration</h3>
						<p class="mb-4">
							Start by creating a configuration file using the <code>ws init</code> command:
						</p>
						<CodeBlock lang="bash" code={`ws init`} />
						<p class="mt-4">
							This will generate a default <code>ws.yaml</code> file that you can customize for your needs.
						</p>
						<div class="alert alert-info mt-6">
							<i class="fa-solid fa-info-circle mr-2"></i>
							<span>
								<strong>Tip:</strong> You can create your own templates (including one named
								<code>default</code>). When you run <code>ws init</code> without a template name,
								withsecrets will use the <code>default</code> template automatically. See
								<a class="link" href="/usage#create-template">Create Template</a>.
							</span>
						</div>
					</div>
				</div>

				<div class="divider">OR</div>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<ClickableHeadline level={3} id="import-from-dotenv" className="card-title"
							>Import from dotenv (.env*)</ClickableHeadline
						>
						<p class="mb-4">
							You can also create a configuration file by importing existing environment variables
							from a <code>.env</code> file using the following command:
						</p>
						<CodeBlock lang="bash" code={`ws convert --from dotenv --infile .env`} />
						<p class="mt-4">
							See <code>ws convert --help</code> for more options on importing from different formats.
						</p>
					</div>
				</div>

				<div class="divider">OR</div>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<ClickableHeadline
							level={3}
							id="import-from-knative-service-ksvc"
							className="card-title">Import from Knative Services (ksvc)</ClickableHeadline
						>
						<p class="mb-4">
							If you're running on Cloud Run / Knative, you can generate a <code>ws.yaml</code>
							from an existing Knative Service manifest. withsecrets will read the container
							<code>env</code> entries, convert hard-coded values to <code>value</code> mappings and
							<code>valueFrom.secretKeyRef</code> entries to <code>secret-key</code> mappings.
						</p>
						<CodeBlock
							lang="bash"
							code={`ws convert --from ksvc --infile service.yaml --env production`}
						/>
						<ClickableHeadline
							level={4}
							id="import-from-knative-service-ksvc-from-deployed-service"
							className="card-title mt-4">Import from already deployed service</ClickableHeadline
						>
						<p class="mt-4">
							You can also import a deployed service directly from your cloud provider (no
							<code>--infile</code> required). This is useful when you want to bootstrap a
							<code>ws.yaml</code> from an existing service definition.
						</p>
						<div class="card bg-base-300">
							<div class="card-body">
								<ClickableHeadline
									level={4}
									id="import-from-deployed-gcp-cloud-run-knative-service-ksvc"
									className="card-title">GCP (Cloud Run)</ClickableHeadline
								>
								<CodeBlock
									lang="bash"
									code={`ws convert --from ksvc \
  --provider gcp \
  --project 1337 \
  --name my-service \
  --env production`}
								/>
							</div>
						</div>
						<div class="card bg-base-300">
							<div class="card-body">
								<ClickableHeadline
									level={4}
									id="import-from-deployed-aws-app-runner-knative-service-ksvc"
									className="card-title">AWS (App Runner)</ClickableHeadline
								>
								<p class="text-sm mb-2">
									For AWS, <code>--name</code> expects <code>service.region</code>.
								</p>
								<CodeBlock
									lang="bash"
									code={`ws convert --from ksvc \
  --provider aws \
  --project 123456789012 \
  --name my-service.us-east-1 \
  --env production`}
								/>
							</div>
						</div>
						<div class="card bg-base-300">
							<div class="card-body">
								<ClickableHeadline
									level={4}
									id="import-from-deployed-azure-container-apps-knative-service-ksvc"
									className="card-title">Azure (Container Apps)</ClickableHeadline
								>
								<p class="text-sm mb-2">
									For Azure, <code>--name</code> expects <code>app.resource-group</code>.
								</p>
								<CodeBlock
									lang="bash"
									code={`ws convert --from ksvc \
  --provider azure \
  --project 00000000-0000-0000-0000-000000000000 \
  --name my-app.my-resource-group \
  --env production`}
								/>
							</div>
						</div>
						<p class="mt-4">
							For Knative Services running on GCP, the environment will default to provider
							<code>gcp</code> and use the Service's <code>metadata.namespace</code> as the
							<code>project</code> (which is typically the GCP project number for Cloud Run).
						</p>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="configuration-file-structure"
					className="text-3xl font-bold mb-6">Configuration File Structure</ClickableHeadline
				>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<h3 class="card-title">Basic Structure</h3>
						<p class="mb-4">
							The <code>ws.yaml</code> file is organized into environment sections, each with its own
							provider and env:
						</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`# yaml-language-server: $schema=https://withsecrets.com/ws.schema.json
---
default:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "database-connection-string"
    API_KEY:
      secret-key: "external-api-key"

development:
  provider: gcp
  project: 1337
  env:
    DEV_DATABASE_URL:
      secret-key: "dev-database-connection-string"

production:
  provider: gcp
  project: 1337
  env:
    PROD_DATABASE_URL:
      secret-key: "prod-database-connection-string"`}
						/>
					</div>
				</div>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Environment Sections</h3>
							<p>
								Each top-level section (like <code>default</code>, <code>development</code>,
								<code>production</code>) represents a different environment configuration.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Provider Configuration</h3>
							<p>
								The <code>provider</code> field specifies which
								<a class="link" href="/providers">provider</a> to use (gcp, aws, azure, openbao, local).
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Project ID</h3>
							<p>
								The <code>project</code> field specifies the project ID for the cloud provider (required
								for GCP and Azure).
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Region</h3>
							<p>
								Optional <code>region</code> (environment or per env item). Used by AWS; when unset,
								falls back to <code>AWS_REGION</code>, then <code>AWS_DEFAULT_REGION</code>.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Env</h3>
							<p>
								The <code>env</code> array defines how secrets are mapped to environment variables.
							</p>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="env-types" className="text-3xl font-bold mb-6"
					>Env Types</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="withsecrets-yaml-env-individual-secrets"
								className="card-title">Individual Secrets (secret-key)</ClickableHeadline
							>
							<p class="mb-4">Fetch a single secret from your cloud provider:</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`env:
  DATABASE_URL:
    secret-key: "database-connection-string"
  API_KEY:
    secret-key: "external-api-key"`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="withsecrets-yaml-env-secret-paths"
								className="card-title">Secret Paths (secret-path)</ClickableHeadline
							>
							<p class="mb-4">
								Fetch all secrets under a path prefix. <code>secret-path</code> (and
								<code>param-path</code>) can be a string or an ordered list of strings; later paths
								overlay earlier ones when names collide:
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`env:
  DB:
    secret-path: "database"
  API:
    secret-path:
      - "external-apis"
      - "external-apis-overrides"`}
							/>
							<p class="mt-4 text-sm">
								This will create environment variables like <code>DB_CONNECTION_STRING</code>,
								<code>DB_USERNAME</code>, <code>API_STRIPE_KEY</code>, etc. (prefix + relative name
								after the path).
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="withsecrets-yaml-env-star-bulk"
								className="card-title">Bulk inject with "*"</ClickableHeadline
							>
							<p class="mb-4">
								Use the literal env key <code>"*"</code> with <code>secret-path</code> or
								<code>param-path</code> to inject each entry under the path as its own environment variable
								(no prefix - just the sanitized path suffix):
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`env:
  "*":
    secret-path:
      - /shared/config
      - /overrides/config`}
							/>
							<p class="mt-4 text-sm">
								Secrets named <code>/shared/config/USERNAME</code> become <code>USERNAME</code>. A
								later path that also defines <code>USERNAME</code> wins.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="withsecrets-yaml-env-param-store"
								className="card-title"
								>AWS Parameter Store (param-key / param-path)</ClickableHeadline
							>
							<p class="mb-4">
								When <code>provider</code> is <code>aws</code>, you can load from Systems Manager
								Parameter Store instead of Secrets Manager:
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`env:
  API_TOKEN:
    param-key: /apps/myapp/API_TOKEN
  "*":
    param-path:
      - /local/withsecrets/config
      - /local/withsecrets/overrides`}
							/>
							<p class="mt-4 text-sm">
								Exactly one of <code>secret-key</code>, <code>secret-path</code>,
								<code>param-key</code>, <code>param-path</code>, or <code>value</code> is allowed
								per env item. <code>param-*</code> is AWS-only.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="withsecrets-yaml-env-hard-coded-values"
								className="card-title">Hard-coded Values (value)</ClickableHeadline
							>
							<p class="mb-4">Set static environment variables:</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`env:
  APP_ENV:
    value: "production"
  DEBUG:
    value: "false"`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="environment-variable-interpolation"
					className="text-3xl font-bold mb-6">Environment Variable Interpolation</ClickableHeadline
				>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<ClickableHeadline
							level={3}
							id="withsecrets-yaml-env-basic-interpolation"
							className="card-title">Basic Interpolation</ClickableHeadline
						>
						<p class="mb-4">
							withsecrets supports environment variable interpolation using <code
								>$&lbrace;VAR_NAME&rbrace;</code
							> syntax:
						</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`env:
  DB_PASSWORD:
    secret-key: "db-password"
  DB_HOST:
    value: "mydbhost"
  DB_CONNECTION_STRING:
    value: "postgresql://user:\${DB_PASSWORD}@\${DB_HOST}:5432/mydb"`}
						/>
					</div>
				</div>

				<div class="card bg-base-200">
					<div class="card-body">
						<ClickableHeadline
							level={3}
							id="withsecrets-yaml-env-system-environment-variables"
							className="card-title">System Environment Variables</ClickableHeadline
						>
						<p>Reference system environment variables:</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`API_URL:
  value: "https://api.\${DOMAIN}/v1"`}
						/>
					</div>
				</div>

				<div class="card bg-base-200">
					<div class="card-body">
						<ClickableHeadline
							level={3}
							id="withsecrets-yaml-env-environment-variables-default-values"
							className="card-title">Default Values</ClickableHeadline
						>
						<p>Provide fallback values with <code>$&lbrace;VAR:-default&rbrace;</code> syntax:</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`REDIS_URL:
  value: "redis://\${REDIS_HOST:-localhost}:\${REDIS_PORT:-6379}/0"`}
						/>
					</div>
				</div>

				<div class="alert alert-info mt-6">
					<i class="fa-solid fa-info-circle mr-2"></i>
					<span>
						<strong>Important:</strong> Interpolation is processed in order, so you can reference variables
						defined earlier in the same configuration.
					</span>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="cross-provider-mappings"
					className="text-3xl font-bold mb-6">Cross-Provider Mappings</ClickableHeadline
				>

				<div class="card bg-base-200 mb-6">
					<div class="card-body">
						<h3 class="card-title">Multiple Cloud Providers</h3>
						<p class="mb-4">
							You can fetch secrets from different cloud providers in the same configuration:
						</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`default:
  provider: gcp
  project: 1337
  env:
    GCP_PROJECT_ID:
      secret-key: "gcp_project_secret"
    AWS_PROJECT_ID:
      secret-key: "aws_project_secret"
      provider: aws
    AZURE_PROJECT_ID:
      secret-key: "azure_project_secret"
      provider: azure
      project: "my-azure-project"`}
						/>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="complete-example" className="text-3xl font-bold mb-6"
					>Complete Example</ClickableHeadline
				>

				<div class="card bg-base-200">
					<div class="card-body">
						<h3 class="card-title">Full Configuration Example</h3>
						<p class="mb-4">Here's a comprehensive example showing all features:</p>
						<CodeBlock
							lang="yaml"
							meta="path=ws.yaml"
							code={`# yaml-language-server: $schema=https://withsecrets.com/ws.schema.json
---
default:
  provider: gcp
  project: 1337
  env:
    # Individual secrets
    DATABASE_URL:
      secret-key: "database-connection-string"
    STRIPE_API_KEY:
      secret-key: "stripe-api-key"

    # Secret paths for bulk loading
    DB:
      secret-path: "database"
    API:
      secret-path: "external-apis"

    # Hard-coded values
    APP_ENV:
      value: "development"
    DEBUG:
      value: "true"

    # Interpolated values
    REDIS_URL:
      value: "redis://\${REDIS_HOST:-localhost$}:\${REDIS_PORT:-6379}/0"
    LOG_LEVEL:
      value: "\${LOG_LEVEL:-info}"

development:
  provider: gcp
  project: 1337
  env:
    DEV_DATABASE_URL:
      secret-key: "dev-database-connection-string"
    DEV_STRIPE_API_KEY:
      secret-key: "dev-stripe-api-key"

staging:
  provider: gcp
  project: 1337
  env:
    STAGING_DATABASE_URL:
      secret-key: "staging-database-connection-string"
    STAGING_STRIPE_API_KEY:
      secret-key: "staging-stripe-api-key"

production:
  provider: gcp
  project: 1337
  env:
    PROD_DATABASE_URL:
      secret-key: "prod-database-connection-string"
    PROD_STRIPE_API_KEY:
      secret-key: "prod-stripe-api-key"
    APP_ENV:
      value: "production"
    DEBUG:
      value: "false"`}
						/>
					</div>
				</div>
			</section>

			<div class="text-center mb-12">
				<ClickableHeadline
					level={1}
					id="withsecrets-global-config"
					className="text-4xl font-bold mb-4">Configure withsecrets itself</ClickableHeadline
				>
				<p class="text-xl text-base-content/70">
					Learn how to configure withsecrets itself (globally).
				</p>
			</div>
			<section>
				<ClickableHeadline
					level={2}
					id="withsecrets-global-templates"
					className="text-3xl font-bold mb-6">withsecrets templates</ClickableHeadline
				>
				<div class="card bg-base-200">
					<div class="card-body">
						<p class="mb-4">
							You can configure the default template that withsecrets uses when you run <code
								>ws init</code
							>
							without a template name. To set the default template, run the following command:
						</p>
						<CodeBlock
							lang="bash"
							code={`ws create template default
`}
						/>
						<p class="mb-4">
							You can also create templates with different names and specify the template to use
							when running
							<code>ws init my-template-name</code>:
						</p>
						<CodeBlock
							lang="bash"
							code={`ws create template my-template-name
`}
						/>
						<p class="mb-4">
							You can also create templates with different names and specify the template to use
							when running
							<code>ws init my-template-name</code>:
						</p>
					</div>
				</div>

				<ClickableHeadline
					level={2}
					id="withsecrets-global-config-defaults"
					className="text-3xl font-bold mb-6 mt-6">withsecrets defaults</ClickableHeadline
				>
				<div class="card bg-base-200">
					<div class="card-body">
						<p class="mb-4">
							There are no defaults set by default. To set defaults for a provider, run the
							following command:
						</p>
						<CodeBlock
							lang="bash"
							code={`ws config defaults set --provider gcp --regions europe-west3
`}
						/>
						<p class="mb-4">
							This will set the default provider to GCP and the default region to <code
								>europe-west3</code
							>. So, when you're using <code>ws tui</code> and add a secret, it will default to
							using <code>europe-west3</code>
							as the region for GCP secrets.
						</p>
						<p class="mb-4">
							Check <code>ws config defaults get --help</code> for more options related to managing the
							defaults.
						</p>
					</div>
				</div>

				<ClickableHeadline
					level={2}
					id="withsecrets-global-config-cache"
					className="text-3xl font-bold mb-6 mt-6">Cache</ClickableHeadline
				>
				<div class="card bg-base-200">
					<div class="card-body">
						<p class="mb-4">
							Caching is off by default. To enable caching, run the following command:
						</p>
						<CodeBlock
							lang="bash"
							code={`ws config cache --enable --ttl 14d
`}
						/>
						<p class="mb-4">
							This will enable caching of secrets locally, with a time-to-live (TTL) of 14 days. You
							can adjust the TTL as needed.
						</p>
						<p class="mb-4">
							Check <code>ws cache --help</code> for more options related to managing the cache.
						</p>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="best-practices" className="text-3xl font-bold mb-6"
					>Best Practices</ClickableHeadline
				>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Organization</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Use descriptive environment variable names</li>
								<li>Group related secrets with secret paths</li>
								<li>Keep environment-specific overrides minimal</li>
								<li>Document your configuration structure</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Security</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Never commit secrets to version control</li>
								<li>Use environment-specific configurations</li>
								<li>Limit access to production secrets</li>
								<li>Rotate secrets regularly</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Maintenance</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Keep configurations in sync across teams</li>
								<li>Use consistent naming conventions</li>
								<li>Test configurations in staging first</li>
								<li>Version control your configuration templates</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Performance</h3>
							<ul class="list-disc list-inside space-y-2">
								<li>Use secret paths for bulk operations</li>
								<li>Avoid unnecessary cross-provider calls</li>
								<li>Cache configurations when possible</li>
								<li>Monitor secret access patterns</li>
							</ul>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="next-steps" className="text-3xl font-bold mb-6"
					>Next Steps</ClickableHeadline
				>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Cloud Providers Setup</h3>
							<p>Configure authentication and permissions for your cloud providers.</p>
							<a href="/providers" class="btn btn-outline bg-lg">Cloud Providers Guide</a>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Usage Examples</h3>
							<p>See practical examples of how to use your configuration.</p>
							<a href="/examples" class="btn btn-outline bg-lg">Examples Guide</a>
						</div>
					</div>
				</div>
			</section>
		</div>
	</div>
</div>
