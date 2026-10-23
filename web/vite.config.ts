import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite-plus';
import { sveltekit } from '@sveltejs/kit/vite';
import { inlineShikiCodeblocks } from './vite-plugins/inline-shiki-codeblocks.js';
import { generateSearchIndexPlugin } from './vite-plugins/generate-search-index.js';

export default defineConfig({
	logLevel: process.env.NODE_ENV === 'production' ? 'silent' : 'info',
	plugins: [generateSearchIndexPlugin(), inlineShikiCodeblocks(), sveltekit(), tailwindcss()]
});
