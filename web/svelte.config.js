import { sveltePreprocess } from 'svelte-preprocess';
import { mdsvex } from 'mdsvex';
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mdsvexShiki } from '@mistweaverco/mdsvex-shiki';

const highlighter = await mdsvexShiki({
	displayLanguage: true,
	displayPath: true
});

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: [vitePreprocess(), sveltePreprocess(), mdsvex({ highlight: { highlighter } })],
	kit: {
		adapter: adapter()
	},
	extensions: ['.svelte', '.svx', '.mdx', '.md']
};

export default config;
