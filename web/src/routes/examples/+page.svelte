<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
	import CodeBlock from '$lib/CodeBlock.svelte';
</script>

<HeadComponent
	data={{
		title: 'Examples - withsecrets',
		description:
			'Practical examples and use cases for using withsecrets with different applications and frameworks.'
	}}
/>

<div class="container mx-auto px-4 py-8">
	<div class="max-w-4xl mx-auto">
		<div class="text-center mb-12">
			<ClickableHeadline level={1} id="examples-and-use-cases" className="text-4xl font-bold mb-4"
				>Examples &amp; Use Cases</ClickableHeadline
			>
			<p class="text-xl text-base-content/70">
				See practical examples of how to use withsecrets with different applications, frameworks,
				and deployment scenarios.
			</p>
		</div>

		<div class="space-y-12">
			<section>
				<ClickableHeadline
					level={2}
					id="web-application-examples"
					className="text-3xl font-bold mb-6">Web Application Examples</ClickableHeadline
				>
				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="nodejs-express-application" className="card-title"
								>Node.js Express Application</ClickableHeadline
							>
							<p class="mb-4">
								Run a Node.js Express application with database credentials and API keys:
							</p>
							<CodeBlock lang="bash" code={`ws run --env production -- node app.js`} />

							<ClickableHeadline
								level={4}
								id="nodejs-withsecrets-configuration"
								className="font-bold mt-4 mb-2 text-left"
								>Configuration (ws.yaml):</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`production:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "prod-database-url"
    JWT_SECRET:
      secret-key: "jwt-secret"
    STRIPE_SECRET_KEY:
      secret-key: "stripe-secret-key"
    REDIS_URL:
      value: "redis://\${REDIS_HOST:-localhost}:6379"`}
							/>

							<ClickableHeadline
								level={4}
								id="nodejs-application-code"
								className="font-bold mt-4 mb-2 text-left">Application Code:</ClickableHeadline
							>
							<CodeBlock
								lang="javascript"
								meta="path=app.js"
								code={`const express = require('express');
const app = express();

// Environment variables are automatically available
const dbUrl = process.env.DATABASE_URL;
const jwtSecret = process.env.JWT_SECRET;
const stripeKey = process.env.STRIPE_SECRET_KEY;
const redisUrl = process.env.REDIS_URL;

console.log('Database URL:', dbUrl);
console.log('Redis URL:', redisUrl);

app.listen(3000, () => {
  console.log('Server running on port 3000');
});`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="python-flask-application" className="card-title"
								>Python Flask Application</ClickableHeadline
							>
							<p class="mb-4">
								Run a Python Flask application with environment-specific configurations:
							</p>
							<CodeBlock lang="bash" code={`ws run --env development -- python app.py`} />

							<ClickableHeadline
								level={4}
								id="python-withsecrets-configuration"
								className="font-bold mt-4 mb-2 text-left"
								>Configuration (ws.yaml):</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`development:
  provider: aws
  env:
    FLASK_ENV:
      value: "development"
    DATABASE_URL:
      secret-key: "dev-database-url"
    SECRET_KEY:
      secret-key: "flask-secret-key"
    DEBUG:
      value: "true"`}
							/>

							<ClickableHeadline
								level={4}
								id="python-application-code"
								className="font-bold mt-4 mb-2 text-left">Application Code:</ClickableHeadline
							>
							<CodeBlock
								lang="python"
								meta="path=app.py"
								code={`from flask import Flask
import os

app = Flask(__name__)

# Environment variables are automatically available
app.config['DATABASE_URL'] = os.environ.get('DATABASE_URL')
app.config['SECRET_KEY'] = os.environ.get('SECRET_KEY')
app.config['DEBUG'] = os.environ.get('DEBUG', 'false').lower() == 'true'

print(f"Database URL: {app.config['DATABASE_URL']}")
print(f"Debug mode: {app.config['DEBUG']}")

if __name__ == '__main__':
    app.run(debug=app.config['DEBUG'])`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="database-and-api-examples"
					className="text-3xl font-bold mb-6">Database Migrations</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="database-migrations" className="card-title"
								>Database Migrations</ClickableHeadline
							>
							<p class="mb-4">Run database migrations with production credentials:</p>
							<CodeBlock
								lang="bash"
								code={`# Run migrations with production database credentials
ws run --env production -- npm run migrate

# Run seed data with development database
ws run --env development -- npm run seed`}
							/>

							<ClickableHeadline
								level={4}
								id="database-migrations-withsecrets-configuraton"
								className="font-bold mt-4 mb-2 text-left"
								>Configuration (ws.yaml):</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`production:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "prod-postgres-url"
    DB_PASSWORD:
      secret-key: "prod-db-password"

development:
  provider: aws
  env:
    DATABASE_URL:
      secret-key: "dev-postgres-url"
    DB_PASSWORD:
      secret-key: "dev-db-password"`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="external-api-integration" className="card-title"
								>External API Integration</ClickableHeadline
							>
							<p class="mb-4">Connect to external APIs with secure keys:</p>
							<CodeBlock lang="bash" code={`ws run --env staging -- python api_client.py`} />

							<ClickableHeadline
								level={4}
								id="external-api-integration-withsecrets-configuration"
								className="font-bold mt-4 mb-2 text-left"
								>Configuration (ws.yaml):</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`staging:
  provider: azure
  env:
    STRIPE_API_KEY:
      secret-key: "stripe-staging-key"
    SENDGRID_API_KEY:
      secret-key: "sendgrid-staging-key"
    TWILIO_ACCOUNT_SID:
      secret-key: "twilio-account-sid"
    TWILIO_AUTH_TOKEN:
      secret-key: "twilio-auth-token"`}
							/>

							<ClickableHeadline
								level={4}
								id="external-api-integration-api-client-code"
								className="font-bold mt-4 mb-2 text-left">API Client Code:</ClickableHeadline
							>
							<CodeBlock
								lang="python"
								meta="path=api_client.py"
								code={`import os
import stripe
import sendgrid
from twilio.rest import Client

# API keys are automatically available
stripe.api_key = os.environ.get('STRIPE_API_KEY')
sendgrid_client = sendgrid.SendGridAPIClient(
    api_key=os.environ.get('SENDGRID_API_KEY')
)
twilio_client = Client(
    os.environ.get('TWILIO_ACCOUNT_SID'),
    os.environ.get('TWILIO_AUTH_TOKEN')
)

print("Stripe API key configured:", bool(stripe.api_key))
print("SendGrid API key configured:", bool(os.environ.get('SENDGRID_API_KEY')))
print("Twilio credentials configured:", bool(os.environ.get('TWILIO_ACCOUNT_SID')))`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="docker-and-container-examples"
					className="text-3xl font-bold mb-6">Docker &amp; Container Examples</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="docker-container-with-secrets" className="card-title"
								>Docker Container with Secrets</ClickableHeadline
							>
							<p class="mb-4">Run Docker containers with environment variables from withsecrets:</p>
							<CodeBlock
								lang="bash"
								code={`# Build image with secrets available during build
ws run --env production -- docker build \
  --build-arg DATABASE_URL \
  --build-arg API_KEY \
  -t myapp .

# Run container with secrets as environment variables
ws run --env production -- docker run \
  -e DATABASE_URL \
  -e API_KEY \
  -e REDIS_URL \
  -p 3000:3000 \
  myapp

# Only pass withsecrets-managed environment variables (not host environment)
docker run --env-file=<(ws show --output dotenv --env production) myapp

# or pass full host environment including withsecrets-managed vars
docker run --env-file=<(ws run --env production -- env) myapp
							`}
							/>

							<ClickableHeadline level={4} id="dockerfile" className="font-bold mt-4 mb-2 text-left"
								>Dockerfile:</ClickableHeadline
							>
							<CodeBlock
								lang="docker"
								meta="path=Dockerfile"
								code={`FROM node:18-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

COPY . .

# Build arguments for secrets
ARG DATABASE_URL
ARG API_KEY

# Set environment variables
ENV DATABASE_URL=$DATABASE_URL
ENV API_KEY=$API_KEY

EXPOSE 3000

CMD ["npm", "start"]`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="docker-compose-integration" className="card-title"
								>Docker Compose Integration</ClickableHeadline
							>
							<p class="mb-4">
								Use withsecrets with Docker Compose for multi-service applications:
							</p>
							<CodeBlock
								lang="bash"
								code={`# Start all services with production secrets
ws run --env production -- docker-compose up -d

# Start specific service with development secrets
ws run --env development -- docker-compose up web`}
							/>

							<ClickableHeadline
								level={4}
								id="docker-compose-yaml"
								className="font-bold mt-4 mb-2 text-left">docker-compose.yml:</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=docker-compose.yml"
								code={`version: '3.8'
services:
  web:
    build: .
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL
      - API_KEY
      - REDIS_URL
    depends_on:
      - db
      - redis

  db:
    image: postgres:15
    environment:
      - POSTGRES_DB=myapp
      - POSTGRES_USER=myapp
      - POSTGRES_PASSWORD
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  postgres_data:`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={2}
					id="ci-cd-pipeline-examples"
					className="text-3xl font-bold mb-6">CI/CD Pipeline Examples</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="github-actions" className="card-title"
								>GitHub Actions</ClickableHeadline
							>
							<p class="mb-4">Integrate withsecrets into GitHub Actions workflows:</p>
							<CodeBlock
								lang="yaml"
								meta="path=.github/workflows/deploy.yaml"
								code={`name: Deploy to Production

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install withsecrets
        run: |
          curl -sSL https://withsecrets.com/install.sh | bash

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: \${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: \${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Build and deploy
        run: |
          ws run --env production -- npm run build
          ws run --env production -- npm run deploy`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="gitlab-ci" className="card-title"
								>GitLab CI</ClickableHeadline
							>
							<p class="mb-4">Use withsecrets in GitLab CI/CD pipelines:</p>
							<CodeBlock
								lang="yaml"
								meta="path=.gitlab-ci.yml"
								code={`stages:
  - test
  - deploy

variables:
  KUBE_CONFIG_FILE: $CI_PROJECT_DIR/ws.yaml

test:
  stage: test
  image: node:18
  before_script:
    - curl -sSL https://withsecrets.com/install.sh | bash
  script:
    - ws run --env testing -- npm test
  only:
    - merge_requests

deploy:
  stage: deploy
  image: node:18
  before_script:
    - curl -sSL https://withsecrets.com/install.sh | bash
  script:
    - ws run --env production -- npm run deploy
  only:
    - main`}
							/>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline
					level={3}
					id="development-workflow-examples"
					className="text-3xl font-bold mb-6">Development Workflow Examples</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="local-development" className="card-title"
								>Local Development</ClickableHeadline
							>
							<p class="mb-4">
								Use withsecrets for local development without managing <code>.env</code> files:
							</p>
							<CodeBlock
								lang="bash"
								code={`# Start development server
ws run --env development -- npm run dev

# Run tests
ws run --env testing -- npm test

# Run database migrations
ws run --env development -- npm run migrate

# Start background services
ws run --env development -- npm run start:services`}
							/>

							<ClickableHeadline
								level={4}
								id="local-development-withsecrets-configuration"
								className="font-bold mt-4 mb-2 text-left"
								>Configuration (ws.yaml):</ClickableHeadline
							>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`development:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "dev-database-url"
    API_KEY:
      secret-key: "dev-api-key"
    DEBUG:
      value: "true"
    LOG_LEVEL:
      value: "debug"

testing:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "test-database-url"
    API_KEY:
      secret-key: "test-api-key"
    NODE_ENV:
      value: "test"`}
							/>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="team-collaboration" className="card-title"
								>Team Collaboration</ClickableHeadline
							>
							<p class="mb-4">Share configuration templates with your team:</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`# ws.yaml (commit this to version control)
default:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "database-url"
    API_KEY:
      secret-key: "api-key"
    REDIS_URL:
      value: "redis://\${REDIS_HOST:-localhost}:6379"

development:
  provider: gcp
  project: 1337
  env:
    DATABASE_URL:
      secret-key: "dev-database-url"
    DEBUG:
      value: "true"`}
							/>

							<p class="mt-4">
								<strong>Instructions for team members:</strong>
							</p>
							<ol class="list-decimal list-inside space-y-2">
								<li>Set up authentication for your cloud provider</li>
								<li>Create the necessary secrets in your cloud provider</li>
								<li>Run <code>ws run --env development -- npm run dev</code></li>
							</ol>
						</div>
					</div>
				</div>
			</section>

			<section>
				<ClickableHeadline level={2} id="advanced-configuration" className="text-3xl font-bold mb-6"
					>Advanced Configuration</ClickableHeadline
				>

				<div class="space-y-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline
								level={3}
								id="multi-environment-with-secret-paths"
								className="card-title">Multi-Environment with Secret Paths</ClickableHeadline
							>
							<p class="mb-4">Use secret paths to bulk-load related secrets:</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`production:
  provider: gcp
  project: 1337
  env:
    # Individual secrets
    APP_ENV:
      value: "production"

    # Database secrets (bulk load)
    DB:
      secret-path: "database"

    # API keys (bulk load)
    API:
      secret-path: "external-apis"

    # Service secrets (bulk load)
    SERVICE:
      secret-path: "microservices"

    # Interpolated connection strings
    DATABASE_URL:
      value: "postgresql://\${DB_USERNAME}:\${DB_PASSWORD}@\${DB_HOST}:\${DB_PORT}/\${DB_NAME}"

    REDIS_URL:
      value: "redis://\${REDIS_HOST:-localhost}:\${REDIS_PORT:-6379}/0"`}
							/>

							<p class="mt-4 text-sm">
								<strong>Note:</strong> This configuration will create environment variables like
								<code>DB_USERNAME</code>, <code>DB_PASSWORD</code>, <code>API_STRIPE_KEY</code>,
								<code>SERVICE_AUTH_TOKEN</code>, etc.
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="aws-parameter-store-bulk" className="card-title"
								>AWS Parameter Store bulk loading</ClickableHeadline
							>
							<p class="mb-4">
								Load every parameter under a path as env vars with <code>"*"</code> and
								<code>param-path</code>:
							</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`default:
  provider: aws
  env:
    "*":
      param-path:
        - /local/withsecrets/config
        - /local/withsecrets/overrides`}
							/>
							<p class="mt-4 text-sm">
								Parameters <code>/local/withsecrets/config/USERNAME</code> and
								<code>/local/withsecrets/config/PASSWORD</code> map to <code>USERNAME</code> and
								<code>PASSWORD</code>. Later paths overlay earlier ones on name collisions. The same
								<code>"*"</code> pattern works with <code>secret-path</code> for Secrets Manager (and
								other providers).
							</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<ClickableHeadline level={3} id="cross-provider-configuration" className="card-title"
								>Cross-Provider Configuration</ClickableHeadline
							>
							<p class="mb-4">Use different cloud providers for different types of secrets:</p>
							<CodeBlock
								lang="yaml"
								meta="path=ws.yaml"
								code={`production:
  provider: gcp
  project: 1337
  env:
    # GCP secrets
    GCP_PROJECT_ID:
      secret-key: "project-id"

    # AWS secrets
    AWS_ACCESS_KEY:
      secret-key: "aws-access-key"
      provider: aws

    # Azure secrets
    AZURE_TENANT_ID:
      secret-key: "tenant-id"
      provider: azure
      project: "my-azure-project"

    # OpenBao secrets
    INTERNAL_API_KEY:
      secret-key: "internal-api-key"
      provider: openbao

    # Hard-coded values
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
				<h2 class="text-3xl font-bold mb-6">Next Steps</h2>

				<div class="grid md:grid-cols-2 gap-6">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Configuration Guide</h3>
							<p>Learn more about advanced configuration options and best practices.</p>
							<a href="/configuration" class="btn btn-outline bg-lg">Configuration Guide</a>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Cloud Providers</h3>
							<p>Set up authentication and permissions for your cloud providers.</p>
							<a href="/providers" class="btn btn-outline bg-lg">Cloud Providers Guide</a>
						</div>
					</div>
				</div>
			</section>
		</div>
	</div>
</div>
