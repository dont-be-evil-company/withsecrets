import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
	extractPageEntries,
	routeFromFile,
	stripMarkup,
	buildSearchIndexFromPages
} from './generate-search-index.js';

const sample = `<script lang="ts">
	import HeadComponent from '$lib/HeadComponent.svelte';
	import ClickableHeadline from '$lib/ClickableHeadline.svelte';
	import CodeBlock from '$lib/CodeBlock.svelte';
</script>

<HeadComponent
	data={{
		title: 'Usage Guide - withsecrets',
		description:
			'Learn how to use withsecrets to run applications with secrets.'
	}}
/>

<ClickableHeadline level={1} id="usage-guide" className="text-4xl"
	>Usage Guide</ClickableHeadline
>
<p>Intro for the page.</p>

<ClickableHeadline level={2} id="tui" className="text-3xl">Interactive TUI</ClickableHeadline>
<p>Use ws tui to view, add, and edit secrets interactively.</p>
<CodeBlock lang="bash" code={\`ws config cache --enable\`} />
`;

describe('generate-search-index', () => {
	it('maps route groups and skips redirects', () => {
		const routesDir = '/docs/src/routes';
		expect(routeFromFile('/docs/src/routes/(index)/+page.svelte', routesDir)).toBe('/');
		expect(routeFromFile('/docs/src/routes/usage/+page.svelte', routesDir)).toBe('/usage');
		expect(routeFromFile('/docs/src/routes/(redirects)/download/+page.svelte', routesDir)).toBe(
			null
		);
	});

	it('strips tags from slot text', () => {
		expect(stripMarkup('Usage <code>Guide</code>')).toBe('Usage Guide');
	});

	it('extracts page and section entries including code', () => {
		const entries = extractPageEntries(sample, '/usage');
		expect(entries[0]).toMatchObject({
			title: 'Usage Guide',
			href: '/usage'
		});
		expect(entries.some((e) => e.href === '/usage#usage-guide')).toBe(false);

		const tui = entries.find((e) => e.href === '/usage#tui');
		expect(tui).toBeTruthy();
		expect(tui?.title).toBe('Interactive TUI');
		expect(tui?.content).toContain('ws config cache --enable');
		expect(`${tui?.excerpt} ${tui?.content}`.toLowerCase()).toContain('ws tui');
	});

	it('indexes files from a routes tree', () => {
		const root = mkdtempSync(join(tmpdir(), 'ws-search-'));
		const usageDir = join(root, 'usage');
		mkdirSync(usageDir, { recursive: true });
		writeFileSync(join(usageDir, '+page.svelte'), sample);
		const entries = buildSearchIndexFromPages(root);
		expect(entries.some((e) => e.href === '/usage#tui')).toBe(true);
	});
});
