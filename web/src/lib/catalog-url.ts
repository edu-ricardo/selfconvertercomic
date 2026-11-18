import type { CatalogFilter, FileStatus, Format } from './api';

/** Lê os filtros do catálogo da URL (assim voltar/avançar e links preservam a busca). */
export function filterFromURL(p: URLSearchParams): CatalogFilter {
	return {
		library: Number(p.get('library')) || undefined,
		publisher: p.get('publisher') || undefined,
		format: (p.get('format') as Format) || '',
		status: (p.get('status') as FileStatus) || '',
		q: p.get('q') || undefined
	};
}

/** Monta a query string dos filtros (mais extras, como a série). */
export function filterToQuery(f: CatalogFilter, extra: Record<string, string> = {}): string {
	const p = new URLSearchParams();
	if (f.library) p.set('library', String(f.library));
	if (f.publisher) p.set('publisher', f.publisher);
	if (f.format) p.set('format', f.format);
	if (f.status) p.set('status', f.status);
	if (f.q) p.set('q', f.q);
	for (const [k, v] of Object.entries(extra)) p.set(k, v);
	const s = p.toString();
	return s ? `?${s}` : '';
}
