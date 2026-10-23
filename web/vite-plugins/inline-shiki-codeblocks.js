import { mdsvexShiki } from '@mistweaverco/mdsvex-shiki';

const highlighterOptions = {
	displayLanguage: true,
	displayPath: true,
	shikiOptions: {
		langs: /** @type {const} */ ([
			'bash',
			'yaml',
			'json',
			'javascript',
			'python',
			'powershell',
			'docker'
		])
	}
};

/** @type {Awaited<ReturnType<Awaited<ReturnType<typeof mdsvexShiki>>>> | undefined} */
let highlight;

async function getHighlight() {
	if (!highlight) {
		const fn = await mdsvexShiki(highlighterOptions);
		highlight = await fn;
	}
	return highlight;
}

function unescapeCodeTemplateSource(raw) {
	return raw.replace(/\\`/g, '`').replace(/\\\$\{/g, '${');
}

const blockRe = /<CodeBlock[\s\S]*?lang="([^"]+)"([\s\S]*?)code=\{`([\s\S]*?)`\}\s*\/>/g;

/**
 * Inlines {@mistweaverco/mdsvex-shiki} output at build time so prerendered HTML
 * contains real highlights.
 * @param {import('rollup').Plugin}
 * @returns {import('rollup').Plugin}
 */
export function inlineShikiCodeblocks() {
	return {
		name: 'inline-shiki-codeblocks',
		// enforce: "pre",
		async transform(code, id) {
			if (!id.includes('/routes/') || !id.endsWith('.svelte')) return null;
			if (!code.includes('<CodeBlock')) return null;

			const h = await getHighlight();
			const matches = [...code.matchAll(blockRe)];
			if (matches.length === 0) return null;

			const replacements = [];
			for (const match of matches) {
				const [full, lang, between, raw] = match;
				const metaMatch = /meta="([^"]*)"/.exec(between);
				const meta = metaMatch ? metaMatch[1] : undefined;
				const source = unescapeCodeTemplateSource(raw);
				const html = await h(source, lang, meta);
				const htmlExpr = '{@html ' + JSON.stringify(html) + '}';
				const replacement = `<div class="mb-0">${htmlExpr}</div>`;
				replacements.push({
					index: match.index,
					len: full.length,
					replacement
				});
			}

			replacements.sort((a, b) => b.index - a.index);
			let out = code;
			for (const { index, len, replacement } of replacements) {
				out = out.slice(0, index) + replacement + out.slice(index + len);
			}

			out = out.replace(/^\s*import CodeBlock from '\$lib\/CodeBlock\.svelte';\n?/m, '');
			out = out.replace(/^\s*import CodeBlock from "\$lib\/CodeBlock\.svelte";\n?/m, '');

			return { code: out, map: null };
		}
	};
}
