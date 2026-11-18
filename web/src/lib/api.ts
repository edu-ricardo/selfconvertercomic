// Cliente da API do servidor Go. Tipos espelham internal/settings.
export type OriginalAction = 'keep' | 'archive' | 'delete';

export interface Settings {
	conversion: {
		autoConvert: boolean;
		stabilityMinutes: number;
		originalAction: OriginalAction;
		archiveDir: string;
		workers: number;
		turboWorkers: number;
		turboHours: number;
	};
	scan: { rescanHours: number };
	notifications: {
		ntfyUrl: string;
		ntfyTopic: string;
		onFinish: boolean;
		onError: boolean;
		onTasks: boolean;
	};
	readers: { kavitaUrl: string; bookOrbitUrl: string };
	metadata: { comicVineApiKey: string; guiaRateSeconds: number; cacheDays: number };
	backup: { daily: boolean; keep: number };
}

/** Erro da API; `fields` traz as mensagens por campo quando a validação falha (422). */
export class ApiError extends Error {
	status: number;
	fields: Record<string, string>;

	constructor(message: string, status: number, fields: Record<string, string> = {}) {
		super(message);
		this.status = status;
		this.fields = fields;
	}
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(path, {
		method,
		headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new ApiError(data.error ?? `HTTP ${res.status}`, res.status, data.fields);
	return data as T;
}

export interface Library {
	id: number;
	name: string;
	path: string;
	autoConvert: boolean;
	available: boolean;
	lastScanAt: string | null;
	lastError: string;
	counts: { pdf: number; cbz: number; cbr: number };
	scanning: boolean;
}

export interface DirListing {
	path: string;
	parent: string;
	dirs: { name: string; path: string }[];
}

export const api = {
	health: () => request<{ status: string; version: string }>('GET', '/api/health'),
	getSettings: () => request<Settings>('GET', '/api/settings'),
	putSettings: (s: Settings) => request<Settings>('PUT', '/api/settings', s),

	listLibraries: () =>
		request<{ libraries: Library[]; watchError: string }>('GET', '/api/libraries'),
	createLibrary: (l: { name: string; path: string; autoConvert: boolean }) =>
		request<Library>('POST', '/api/libraries', l),
	updateLibrary: (id: number, l: { name: string; autoConvert: boolean }) =>
		request<Library>('PUT', `/api/libraries/${id}`, l),
	deleteLibrary: (id: number) => request<void>('DELETE', `/api/libraries/${id}`),
	scanLibrary: (id: number) => request<{ queued: boolean }>('POST', `/api/libraries/${id}/scan`),
	browse: (path = '') => request<DirListing>('GET', `/api/fs?path=${encodeURIComponent(path)}`)
};
