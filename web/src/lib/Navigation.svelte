<script lang="ts">
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { SEARCH_INDEX } from '$lib/searchIndex';

	const gotoWrapper = (href: string) => {
		if (browser) {
			window.location.href = href;
		} else {
			goto(href);
		}
	};

	$: currentPath = $page.url.pathname;

	let searchQuery = '';
	let searchOpen = false;
	let selectedIndex = -1;
	$: normalizedQuery = searchQuery.trim().toLowerCase();
	$: searchResults =
		normalizedQuery.length < 2
			? []
			: SEARCH_INDEX.filter((e) => {
					const haystack =
						`${e.title} ${e.href} ${e.excerpt ?? ''} ${e.keywords.join(' ')} ${e.content ?? ''}`.toLowerCase();
					return haystack.includes(normalizedQuery);
				}).slice(0, 12);
	$: selectedIndex =
		searchResults.length === 0
			? -1
			: Math.min(Math.max(selectedIndex, 0), searchResults.length - 1);
	$: if (searchOpen && searchResults.length > 0 && selectedIndex === -1) {
		selectedIndex = 0;
	}

	function closeSearch() {
		searchOpen = false;
		selectedIndex = -1;
	}

	function openSelected() {
		if (searchResults.length === 0) return;
		const idx = selectedIndex >= 0 ? selectedIndex : 0;
		if (idx >= searchResults.length) return;
		const href = searchResults[idx].href;
		gotoWrapper(href);
		searchQuery = '';
		closeSearch();
	}

	const navItems = [
		{ href: '/', label: 'Home' },
		{ href: '/installation', label: 'Installation' },
		{ href: '/usage', label: 'Usage' },
		{ href: '/configuration', label: 'Configuration' },
		{ href: '/providers', label: 'Providers' },
		{ href: '/examples', label: 'Examples' }
	];
</script>

<nav class="navbar bg-base-100 shadow-lg">
	<div class="navbar-start">
		<div class="dropdown">
			<div tabindex="0" role="button" class="btn btn-ghost lg:hidden">
				<i class="fa-solid fa-bars"></i>
			</div>
			<ul class="menu menu-sm dropdown-content mt-3 z-[1] p-2 shadow bg-base-100 rounded-box w-52">
				{#each navItems as item}
					<li>
						<a href={item.href} class={currentPath === item.href ? 'active' : ''}>
							{item.label}
						</a>
					</li>
				{/each}
			</ul>
		</div>
		<a href="/" class="btn btn-ghost text-xl">
			<img src="/logo.svg" alt="withsecrets" class="w-8 h-8 mr-2" />
			<span class="hidden sm:inline"> withsecrets </span>
		</a>
	</div>
	<div class="navbar-center hidden lg:flex">
		<ul class="menu menu-horizontal px-1">
			{#each navItems as item}
				<li>
					<a href={item.href} class={currentPath === item.href ? 'active' : ''}>
						{item.label}
					</a>
				</li>
			{/each}
		</ul>
	</div>
	<div class="navbar-end">
		<div class="flex items-center mr-2 relative">
			<input
				class="input input-bordered input-sm max-w-56"
				type="search"
				placeholder="Search docs..."
				bind:value={searchQuery}
				on:focus={() => (searchOpen = true)}
				on:keydown={(e) => {
					if (e.key === 'Escape') return closeSearch();
					if (!searchOpen) return;

					if (e.key === 'ArrowDown') {
						e.preventDefault();
						if (searchResults.length === 0) return;
						selectedIndex = selectedIndex < 0 ? 0 : (selectedIndex + 1) % searchResults.length;
						return;
					}
					if (e.key === 'ArrowUp') {
						e.preventDefault();
						if (searchResults.length === 0) return;
						selectedIndex =
							selectedIndex < 0
								? searchResults.length - 1
								: (selectedIndex - 1 + searchResults.length) % searchResults.length;
						return;
					}
					if (e.key === 'Enter') {
						e.preventDefault();
						openSelected();
						return;
					}
				}}
				on:blur={() => {
					// only close if the new focused element isn't part of the search results
					setTimeout(() => {
						const activeEl = document.activeElement;
						if (
							activeEl &&
							(activeEl.id === 'doc-search-results' ||
								activeEl.closest('#doc-search-results')?.contains(activeEl))
						) {
							return;
						}
						closeSearch();
					}, 100);
				}}
				aria-label="Search documentation"
				aria-expanded={searchOpen && searchResults.length > 0}
				aria-controls="doc-search-results"
				aria-activedescendant={selectedIndex >= 0
					? `doc-search-option-${selectedIndex}`
					: undefined}
				role="combobox"
			/>
			{#if searchOpen && searchResults.length > 0}
				<div
					class="absolute right-0 top-10 z-[20] w-80 rounded-box bg-base-100 shadow p-2 border border-base-300"
				>
					<ul id="doc-search-results" class="menu" role="listbox">
						{#each searchResults as r, idx}
							<li>
								<a
									id={`doc-search-option-${idx}`}
									href={r.href}
									class={idx === selectedIndex ? 'bg-primary text-primary-content active' : ''}
									on:click={(evt) => {
										evt.preventDefault();
										openSelected();
									}}
									on:mouseenter={() => {
										console.log('hover', idx);
										selectedIndex = idx;
									}}
									role="option"
									aria-selected={idx === selectedIndex}
								>
									<div class="flex flex-col">
										<span class="font-semibold">{r.title}</span>
										{#if r.excerpt}
											<span class="text-xs opacity-70">{r.excerpt}</span>
										{/if}
									</div>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
		<a
			href="https://github.com/dont-be-evil-company/withsecrets"
			class="btn btn-ghost btn-circle"
			aria-label="View withsecrets on GitHub"
		>
			<i class="fa-brands fa-github fa-lg"></i>
		</a>
	</div>
</nav>
