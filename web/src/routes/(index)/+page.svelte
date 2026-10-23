<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
</script>

<HeadComponent
	data={{
		title: 'withsecrets - Securely and easily access your environment variables',
		description:
			'Pass env directly from GCP Secret Manager, AWS Secrets Manager, AWS Parameter Store, Azure Key Vault, OpenBao, Bitwarden, and more to your application.'
	}}
/>

<div class="hero bg-base-200 min-h-screen">
	<div class="hero-content text-center">
		<div class="max-w-4xl">
			<img src="/logo.svg" alt="withsecrets" class="m-5 mx-auto w-64" />
			<h1 class="text-5xl font-bold">withsecrets</h1>
			<p class="py-2 text-2xl font-mono text-base-content/80">ws</p>
			<p class="py-6 text-xl">Securely and easily access your environment variables</p>
			<p class="py-4">
				withsecrets helps you get rid of <code>.env</code> files. Pass env directly from GCP Secret Manager,
				AWS Secrets Manager, AWS Parameter Store, Azure Key Vault, OpenBao, Bitwarden Secrets Manager, and more to your application.
			</p>
			<div class="flex gap-4 justify-center mt-8">
				<a href="/installation" class="btn btn-primary">Get Started</a>
				<a href="/usage" class="btn btn-secondary">Learn More</a>
			</div>
		</div>
	</div>
</div>

<div class="py-16 bg-base-100">
	<div class="container mx-auto px-4">
		<ClickableHeadline
			level={2}
			id="why-choose-withsecrets"
			className="text-3xl font-bold text-center mb-12">Why Choose withsecrets?</ClickableHeadline
		>
		<div class="grid md:grid-cols-2 lg:grid-cols-3 gap-8">
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">🔒 Secure</h3>
					<p>
						Eliminate insecure <code>.env</code> files and leverage your existing cloud provider security
					</p>
				</div>
			</div>
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">🚀 Simple</h3>
					<p>Single binary (<code>ws</code>) with a simple YAML configuration file</p>
				</div>
			</div>
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">☁️ Multi-Cloud</h3>
					<p>Support for GCP, AWS, Azure, OpenBao, Bitwarden, and local values all in one tool</p>
				</div>
			</div>
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">💰 Free</h3>
					<p>No subscription fees - leverage your existing cloud infrastructure</p>
				</div>
			</div>
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">⚡ Fast</h3>
					<p>Single binary written in Go for optimal performance</p>
				</div>
			</div>
			<div class="card bg-base-200 shadow-xl">
				<div class="card-body">
					<h3 class="card-title">🔧 Flexible</h3>
					<p>Environment-specific configurations with variable interpolation</p>
				</div>
			</div>
		</div>
	</div>
</div>

<div class="py-16 bg-base-200">
	<div class="container mx-auto px-4">
		<ClickableHeadline level={2} id="quick-start" className="text-3xl font-bold text-center mb-12"
			>Quick Start</ClickableHeadline
		>
		<div class="grid md:grid-cols-3 gap-8">
			<div class="text-center">
				<div class="text-4xl mb-4">1️⃣</div>
				<h3 class="text-xl font-bold mb-4">Install</h3>
				<p class="mb-4">Download and install <code>ws</code> on your system</p>
				<a href="/installation" class="btn btn-primary">Installation Guide</a>
			</div>
			<div class="text-center">
				<div class="text-4xl mb-4">2️⃣</div>
				<h3 class="text-xl font-bold mb-4">Configure</h3>
				<p class="mb-4">Set up your <code>ws.yaml</code> configuration file</p>
				<a href="/configuration" class="btn btn-primary">Configuration Guide</a>
			</div>
			<div class="text-center">
				<div class="text-4xl mb-4">3️⃣</div>
				<h3 class="text-xl font-bold mb-4">Run</h3>
				<p class="mb-4">Execute your applications with secure environment variables</p>
				<a href="/usage" class="btn btn-primary">Usage Guide</a>
			</div>
		</div>
	</div>
</div>

<div class="py-16 bg-base-100">
	<div class="container mx-auto px-4 text-center">
		<ClickableHeadline level={2} id="ready-to-get-started" className="text-3xl font-bold mb-8"
			>Ready to Get Started?</ClickableHeadline
		>
		<p class="text-lg mb-8">
			Join the community and start managing your secrets securely with withsecrets.
		</p>
		<div class="flex gap-4 justify-center">
			<a href="/installation" class="btn btn-primary btn-lg">Install ws</a>
			<a href="https://github.com/dont-be-evil-company/withsecrets" class="btn btn-outline btn-lg"
				>View on GitHub</a
			>
		</div>
	</div>
</div>
