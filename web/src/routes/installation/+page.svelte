<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
	import CodeBlock from '$lib/CodeBlock.svelte';

	let installUsing = 'manual';

	const handleInstallUsingChange = (evt: Event) => {
		const select = evt.currentTarget as HTMLSelectElement;
		installUsing = select.value;
	};
</script>

<HeadComponent
	data={{
		title: 'Installation - withsecrets',
		description: 'Install withsecrets on Linux, macOS, and Windows using various methods.'
	}}
/>

<div class="container mx-auto px-4 py-8">
	<div class="max-w-4xl mx-auto">
		<div class="text-center mb-12">
			<ClickableHeadline level={1} id="installation-guide" className="text-4xl font-bold mb-4"
				>Installation Guide</ClickableHeadline
			>
			<p class="text-xl text-base-content/70">
				Get withsecrets up and running on your system with these simple installation methods.
			</p>
		</div>

		<div class="grid lg:grid-cols-2 gap-8">
			<div>
				<ClickableHeadline level={2} id="automatic-installation" className="text-2xl font-bold mb-6"
					>Automatic Installation</ClickableHeadline
				>

				<div class="mb-6">
					<label for="install-method" class="block text-sm font-medium mb-2">
						Choose your installation method:
					</label>
					<select
						id="install-method"
						bind:value={installUsing}
						on:change={handleInstallUsingChange}
						class="select select-bordered w-full"
					>
						<option value="manual">Manual Installation</option>
						<option value="curl-zsh">curl & zsh (Linux/macOS)</option>
						<option value="curl-bash">curl & bash (Linux/macOS)</option>
						<option value="wget-zsh">wget & zsh (Linux/macOS)</option>
						<option value="wget-bash">wget & bash (Linux/macOS)</option>
						<option value="arch-aur">Arch Linux (AUR)</option>
						<option value="arch-pkgbuild">Arch Linux (PKGBUILD)</option>
						<option value="pwsh">PowerShell (Windows)</option>
					</select>
				</div>

				<div class="space-y-4">
					<div class="card bg-base-200 {installUsing === 'curl-zsh' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">curl & zsh (Linux/macOS)</h3>
							<CodeBlock lang="bash" code={`curl -sSL https://withsecrets.com/install.sh | zsh`} />
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'curl-bash' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">curl & bash (Linux/macOS)</h3>
							<CodeBlock lang="bash" code={`curl -sSL https://withsecrets.com/install.sh | bash`} />
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'wget-zsh' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">wget & zsh (Linux/macOS)</h3>
							<CodeBlock lang="bash" code={`wget -qO- https://withsecrets.com/install.sh | zsh`} />
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'wget-bash' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">wget & bash (Linux/macOS)</h3>
							<CodeBlock lang="bash" code={`wget -qO- https://withsecrets.com/install.sh | bash`} />
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'arch-aur' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">Arch Linux (AUR)</h3>
							<p class="mb-4">
								withsecrets is available in the AUR as <a
									href="https://aur.archlinux.org/packages/withsecrets-bin"
									class="link link-primary">withsecrets-bin</a
								>.
							</p>
							<CodeBlock lang="bash" code={`paru -S withsecrets-bin`} />
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'arch-pkgbuild' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">Arch Linux (PKGBUILD)</h3>
							<p class="mb-4">
								withsecrets ships a <code>PKGBUILD</code> you can use to build and install a
								<code>withsecrets-bin</code>
								package from GitHub release binaries.
							</p>
							<CodeBlock
								lang="bash"
								code={`# One-time setup (build tools)
sudo pacman -S --needed base-devel git

# Build from the PKGBUILD in the repo
git clone https://github.com/dont-be-evil-company/withsecrets.git
cd withsecrets/scripts

# Build & install the package
makepkg -si`}
							/>
							<div class="alert alert-info mt-4">
								<i class="fa-solid fa-info-circle mr-2"></i>
								<span>
									The package name is <code>withsecrets-bin</code> and installs the binary as
									<code>ws</code>. If you maintain a custom repo / AUR workflow, you can adapt the
									PKGBUILD.
								</span>
							</div>
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'pwsh' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">PowerShell (Windows)</h3>
							<CodeBlock
								lang="powershell"
								code={`iwr https://withsecrets.com/install.ps1 -useb | iex`}
							/>
						</div>
					</div>
					<div class="card bg-base-200 {installUsing === 'manual' ? '' : 'hidden'}">
						<div class="card-body">
							<h3 class="card-title">Manual Installation</h3>
							<p class="mb-4">
								Download the latest release from the <a href="/download" class="link link-primary"
									>download page</a
								>.
							</p>
							<p>
								Or visit our <a
									href="https://github.com/dont-be-evil-company/withsecrets/releases/latest"
									class="link link-primary">GitHub releases</a
								> page.
							</p>
						</div>
					</div>
				</div>
			</div>

			<div>
				<ClickableHeadline level={2} id="system-requirements" className="text-2xl font-bold mb-6"
					>System Requirements</ClickableHeadline
				>

				<div class="space-y-4">
					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Operating Systems</h3>
							<ul class="list-disc list-inside space-y-2">
								<li><strong>Linux:</strong> x86_64, ARM64, ARMv7</li>
								<li><strong>macOS:</strong> Intel, Apple Silicon (M1/M2)</li>
								<li><strong>Windows:</strong> x86_64</li>
							</ul>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Dependencies</h3>
							<p>withsecrets is a single binary with no external dependencies required.</p>
						</div>
					</div>

					<div class="card bg-base-200">
						<div class="card-body">
							<h3 class="card-title">Network Access</h3>
							<p>Required for downloading releases and accessing cloud provider APIs.</p>
						</div>
					</div>
				</div>
			</div>
		</div>

		<div class="mt-12">
			<ClickableHeadline level={2} id="verification" className="text-2xl font-bold mb-6"
				>Verification</ClickableHeadline
			>

			<div class="card bg-base-200">
				<div class="card-body">
					<p class="mb-4">After installation, verify that withsecrets is working correctly:</p>
					<CodeBlock lang="bash" code={`ws --version`} />
					<p class="mt-4 text-sm text-base-content/70">
						You should see the current version of withsecrets displayed.
					</p>
				</div>
			</div>
		</div>

		<div class="mt-12">
			<ClickableHeadline level={2} id="next-steps" className="text-2xl font-bold mb-6"
				>Next Steps</ClickableHeadline
			>

			<div class="grid md:grid-cols-2 gap-6">
				<div class="card bg-base-200">
					<div class="card-body">
						<h3 class="card-title">Configure withsecrets</h3>
						<p>Set up your configuration file to start using withsecrets with your cloud providers.</p>
						<a href="/configuration" class="btn btn-outline bg-lg">Configuration Guide</a>
					</div>
				</div>

				<div class="card bg-base-200">
					<div class="card-body">
						<h3 class="card-title">Learn Usage</h3>
						<p>
							Discover how to use withsecrets to run your applications with secure environment variables.
						</p>
						<a href="/usage" class="btn btn-outline bg-lg">Usage Guide</a>
					</div>
				</div>
			</div>
		</div>

		<div class="mt-12 text-center">
			<div class="alert alert-info">
				<i class="fa-solid fa-info-circle mr-2"></i>
				<span>
					<strong>Need help?</strong> Check out our
					<a href="https://github.com/dont-be-evil-company/withsecrets/issues" class="link">GitHub issues</a>
					or join our <a href="https://mistweaverco.com/discord" class="link">Discord community</a>.
				</span>
			</div>
		</div>
	</div>
</div>
