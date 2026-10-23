import { GENERATED_SEARCH_INDEX } from './searchIndex.generated';

export type SearchEntry = {
	title: string;
	href: string;
	/** Extra terms users might type */
	keywords: string[];
	/** Short hint shown in results */
	excerpt?: string;
	/** Full-text body used for matching, not shown in the dropdown */
	content?: string;
};

/** Optional synonyms merged onto generated entries by href. */
export const SEARCH_EXTRAS: Record<string, Pick<SearchEntry, 'keywords'> & { excerpt?: string }> = {
	'/installation': {
		keywords: [
			'aur',
			'paru',
			'yay',
			'withsecrets-bin',
			'pkgbuild',
			'pacman',
			'makepkg',
			'powershell'
		]
	},
	'/usage': {
		keywords: ['contain', 'ci', 'cd']
	},
	'/configuration': {
		keywords: ['ws.yaml', 'param-key', 'param-path', 'secret-key', 'secret-path']
	}
};

function mergeKeywords(base: readonly string[], extra?: string[]) {
	if (!extra?.length) return [...base];
	const seen = new Set(base.map((k) => k.toLowerCase()));
	const out = [...base];
	for (const keyword of extra) {
		const key = keyword.toLowerCase();
		if (seen.has(key)) continue;
		seen.add(key);
		out.push(keyword);
	}
	return out;
}

export const SEARCH_INDEX: SearchEntry[] = GENERATED_SEARCH_INDEX.map((entry) => {
	const extra = SEARCH_EXTRAS[entry.href];
	return {
		title: entry.title,
		href: entry.href,
		keywords: mergeKeywords(entry.keywords, extra?.keywords),
		excerpt: extra?.excerpt ?? entry.excerpt,
		content: entry.content
	};
});
