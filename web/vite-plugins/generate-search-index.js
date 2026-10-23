import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const pluginDir = dirname(fileURLToPath(import.meta.url));
const defaultWebRoot = dirname(pluginDir);

const headlineRe = /<ClickableHeadline\b([^>]*)>([\s\S]*?)<\/ClickableHeadline\s*>/g;
const codeBlockRe = /<CodeBlock[\s\S]*?code=\{`([\s\S]*?)`\}/g;
const paragraphRe = /<p\b[^>]*>([\s\S]*?)<\/p>/i;
const h1Re = /<h1\b[^>]*>([\s\S]*?)<\/h1>/i;

/**
 * @param {string} raw
 */
function unescapeCodeTemplateSource(raw) {
	return raw.replace(/\\`/g, '`').replace(/\\\$\{/g, '${');
}

/**
 * @param {string} value
 */
export function stripMarkup(value) {
	return value
		.replace(/\{[#:/@][\s\S]*?\}/g, ' ')
		.replace(/<[^>]+>/g, ' ')
		.replace(/\{[^}]+\}/g, ' ')
		.replace(/&amp;/g, '&')
		.replace(/&lt;/g, '<')
		.replace(/&gt;/g, '>')
		.replace(/&quot;/g, '"')
		.replace(/&#39;/g, "'")
		.replace(/\s+/g, ' ')
		.trim();
}

/**
 * @param {string} text
 * @param {number} max
 */
function excerptFrom(text, max = 180) {
	const trimmed = text.replace(/\s+/g, ' ').trim();
	if (trimmed.length <= max) return trimmed;
	return trimmed.slice(0, max).replace(/\s+\S*$/, '') + '...';
}

/**
 * @param {string} title
 */
function keywordsFromTitle(title) {
	const seen = new Set();
	const out = [];
	for (const word of title.toLowerCase().split(/[^a-z0-9]+/)) {
		if (word.length < 2 || seen.has(word)) continue;
		seen.add(word);
		out.push(word);
	}
	return out;
}

/**
 * @param {string} source
 * @param {string} key
 */
function extractQuoted(source, key) {
	const re = new RegExp(`${key}:\\s*(['"\`])([\\s\\S]*?)\\1`);
	const match = source.match(re);
	return match ? match[2].replace(/\s+/g, ' ').trim() : '';
}

/**
 * @param {string} file
 * @param {string} routesDir
 */
export function routeFromFile(file, routesDir) {
	const rel = relative(routesDir, dirname(file)).replaceAll('\\', '/');
	if (!rel || rel.startsWith('..')) return null;
	const segments = rel.split('/').filter(Boolean);
	if (segments.some((part) => part === '(redirects)')) return null;
	const parts = segments.filter((part) => !/^\(.*\)$/.test(part));
	return parts.length === 0 ? '/' : `/${parts.join('/')}`;
}

/**
 * @param {string} dir
 * @returns {string[]}
 */
function walkPageFiles(dir) {
	/** @type {string[]} */
	const out = [];
	for (const ent of readdirSync(dir, { withFileTypes: true })) {
		const path = join(dir, ent.name);
		if (ent.isDirectory()) {
			out.push(...walkPageFiles(path));
		} else if (ent.name === '+page.svelte') {
			out.push(path);
		}
	}
	return out;
}

/**
 * @param {string} attrs
 * @param {string} name
 */
function attrValue(attrs, name) {
	const match = attrs.match(new RegExp(`\\b${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|\\{([^}]*)\\})`));
	return match ? (match[1] ?? match[2] ?? match[3] ?? '').trim() : '';
}

/**
 * @param {string} source
 * @param {string} route
 */
export function extractPageEntries(source, route) {
	const headTitle = extractQuoted(source, 'title');
	const headDescription = extractQuoted(source, 'description');
	const h1Text = stripMarkup((source.match(h1Re) ?? [])[1] ?? '');

	/** @type {{ id: string, level: number, title: string, index: number, end: number }[]} */
	const headlines = [];
	for (const match of source.matchAll(headlineRe)) {
		const attrs = match[1] ?? '';
		const id = attrValue(attrs, 'id');
		if (!id) continue;
		const levelRaw = attrValue(attrs, 'level').replace(/[{}]/g, '');
		const level = Number.parseInt(levelRaw, 10) || 2;
		const title = stripMarkup(match[2] ?? '');
		if (!title) continue;
		headlines.push({
			id,
			level,
			title,
			index: match.index ?? 0,
			end: (match.index ?? 0) + match[0].length
		});
	}

	const firstH1 = headlines.find((h) => h.level === 1);
	let pageTitle = firstH1?.title || h1Text;
	if (route === '/') {
		pageTitle = 'Home';
	} else if (!pageTitle && headTitle) {
		pageTitle = headTitle.replace(/\s*-\s*withsecrets\s*$/i, '').trim();
	} else if (!pageTitle) {
		pageTitle = route.replace(/^\//, '') || 'Home';
	}

	/** @type {{ title: string, href: string, keywords: string[], excerpt?: string, content?: string }[]} */
	const entries = [
		{
			title: pageTitle,
			href: route,
			keywords: keywordsFromTitle(pageTitle),
			excerpt: excerptFrom(headDescription || h1Text || pageTitle),
			content: stripMarkup(`${headTitle} ${headDescription}`)
		}
	];

	for (let i = 0; i < headlines.length; i++) {
		const headline = headlines[i];
		if (headline.level === 1) continue;

		const sectionEnd = i + 1 < headlines.length ? headlines[i + 1].index : source.length;
		const section = source.slice(headline.end, sectionEnd);
		const paragraph = stripMarkup((section.match(paragraphRe) ?? [])[1] ?? '');
		const codes = [...section.matchAll(codeBlockRe)].map((m) =>
			unescapeCodeTemplateSource(m[1] ?? '')
		);
		const prose = stripMarkup(section);
		const content = [prose, ...codes].filter(Boolean).join(' ');

		entries.push({
			title: headline.title,
			href: `${route}#${headline.id}`,
			keywords: keywordsFromTitle(headline.title),
			excerpt: excerptFrom(paragraph || prose || headline.title),
			content
		});
	}

	return entries;
}

/**
 * @param {string} routesDir
 */
export function buildSearchIndexFromPages(routesDir) {
	const files = walkPageFiles(routesDir).sort();
	/** @type {{ title: string, href: string, keywords: string[], excerpt?: string, content?: string }[]} */
	const entries = [];
	for (const file of files) {
		const route = routeFromFile(file, routesDir);
		if (!route) continue;
		const source = readFileSync(file, 'utf8');
		entries.push(...extractPageEntries(source, route));
	}
	return entries;
}

/**
 * @param {string} [webRoot]
 */
export function writeSearchIndex(webRoot = defaultWebRoot) {
	const routesDir = join(webRoot, 'src/routes');
	const outFile = join(webRoot, 'src/lib/searchIndex.generated.ts');
	const entries = buildSearchIndexFromPages(routesDir);
	const body =
		'// This file is generated by vite-plugins/generate-search-index.js. Do not edit.\n' +
		`export const GENERATED_SEARCH_INDEX = ${JSON.stringify(entries, null, '\t')} as const;\n`;
	writeFileSync(outFile, body);
	return { outFile, count: entries.length };
}

/**
 * @param {{ root?: string }} [options]
 * @returns {import("vite").Plugin}
 */
export function generateSearchIndexPlugin(options = {}) {
	let root = options.root ?? '';
	return {
		name: 'generate-search-index',
		configResolved(config) {
			root = options.root ?? config.root;
		},
		buildStart() {
			writeSearchIndex(root || defaultWebRoot);
		},
		handleHotUpdate({ file }) {
			const normalized = file.replaceAll('\\', '/');
			if (normalized.includes('/routes/') && normalized.endsWith('+page.svelte')) {
				writeSearchIndex(root || defaultWebRoot);
			}
		}
	};
}

const invoked = process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href;
if (invoked) {
	const result = writeSearchIndex();
	console.log(`Wrote ${result.count} search entries to ${result.outFile}`);
}
