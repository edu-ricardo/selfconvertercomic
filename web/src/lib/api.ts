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
	metadata: {
		autoIdentify: boolean;
		comicVineApiKey: string;
		gcdEnabled: boolean;
		rateSeconds: number;
		cacheDays: number;
	};
	organize: { template: string };
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

export type JobStatus = 'queued' | 'running' | 'done' | 'failed' | 'cancelled';
export type JobFilter = 'active' | 'failed' | 'done' | 'cancelled' | 'all';

export interface Job {
	id: number;
	libraryId: number;
	libraryName: string;
	relPath: string;
	srcSize: number;
	auto: boolean;
	status: JobStatus;
	progress: number;
	pages: number;
	extracted: number;
	rendered: number;
	outputRelPath: string;
	error: string;
	attempts: number;
	createdAt: string;
	startedAt: string | null;
	finishedAt: string | null;
}

export interface Turbo {
	enabled: boolean;
	until: string | null;
}

export type FileStatus = 'pending' | 'converting' | 'failed' | 'converted' | 'ready';
export type Format = 'pdf' | 'cbz' | 'cbr';

export interface CatalogFilter {
	library?: number;
	publisher?: string;
	format?: Format | '';
	status?: FileStatus | '';
	q?: string;
	offset?: number;
	limit?: number;
}

export interface Series {
	libraryId: number;
	libraryName: string;
	key: string;
	name: string;
	publisher: string;
	count: number;
	formats: Record<Format, number>;
	coverFileId: number;
}

export interface CatalogFile {
	id: number;
	libraryId: number;
	libraryName: string;
	relPath: string;
	format: Format;
	size: number;
	mtime: number;
	publisher: string;
	series: string; // chave da série (a mesma de Series.key)
	seriesName: string;
	volume: string;
	number: string; // "" = desconhecido
	numberInferred: boolean; // deduzido pela ordem de leitura
	year: number;
	title: string;
	status: FileStatus;
	editionId: number; // 0 = ainda sem edição
	metaStatus: MetaStatus;
}

export type MetaStatus = '' | 'new' | 'queued' | 'applied' | 'rewrite' | 'task' | 'error';

export interface Candidate {
	provider: 'gcd' | 'comicvine';
	issueId: string;
	seriesId: string;
	series: string;
	number: string;
	title: string;
	year: number;
	publisher: string;
	language: string;
	issueCount: number;
	coverUrl: string;
	url: string;
	score: number;
}

export interface MetadataDetails {
	provider: 'gcd' | 'comicvine';
	issueId: string;
	url: string;
	series: string;
	number: string;
	title: string;
	year: number;
	month: number;
	publisher: string;
	language: string;
	writers: string[] | null;
	pencillers: string[] | null;
	inkers: string[] | null;
	colorists: string[] | null;
	letterers: string[] | null;
	coverArtists: string[] | null;
	summary: string;
	coverUrl: string;
}

export interface EditionMeta {
	id: number;
	libraryId: number;
	key: string;
	guess: { series: string; volume: string; number: string; year: number };
	status: MetaStatus;
	error: string;
	score: number;
	applied: MetadataDetails | null;
	candidates: Candidate[];
}

export interface Page<T> {
	items: T[];
	total: number;
}

function query(f: CatalogFilter & { series?: string }): string {
	const p = new URLSearchParams();
	for (const [k, v] of Object.entries(f)) {
		if (v !== undefined && v !== '' && v !== 0) p.set(k, String(v));
	}
	if (f.series !== undefined) p.set('series', f.series); // "" = raiz, é um valor válido
	return p.toString();
}

/** URL da miniatura; v=mtime invalida o cache do navegador quando o arquivo muda. */
export const thumbUrl = (id: number, mtime?: number) => `/api/files/${id}/thumb${mtime ? `?v=${mtime}` : ''}`;

export type TaskStatus = 'open' | 'resolved' | 'dismissed';

export interface Task {
	id: number;
	kind: string;
	libraryId: number;
	libraryName: string;
	relPath: string;
	refId: number;
	title: string;
	detail: string;
	status: TaskStatus;
	createdAt: string;
	resolvedAt: string | null;
}

export interface OrganizeItem {
	editionId: number;
	from: string;
	to: string;
	source: 'fontes' | 'nome';
	problem: string;
	files: { fileId: number; from: string; to: string }[];
}

export interface OrganizePlan {
	template: string;
	items: OrganizeItem[];
	unchanged: number;
}

export interface OrganizeOp {
	id: number;
	libraryId: number;
	template: string;
	status: 'done' | 'partial' | 'undone' | 'running';
	error: string;
	moves: number;
	editions: number;
	createdAt: string;
	undoneAt: string | null;
}

export interface DuplicateFile {
	id: number;
	libraryId: number;
	libraryName: string;
	relPath: string;
	format: Format;
	size: number;
	editionId: number;
}

export interface DuplicateGroup {
	kind: 'content' | 'edition';
	label: string;
	files: DuplicateFile[];
}

export interface DuplicatesReport {
	pending: number;
	total: number;
	groups: DuplicateGroup[];
	more: boolean;
}

export interface BackupInfo {
	name: string;
	size: number;
	createdAt: string;
}

async function restoreBackup(file: File) {
	const res = await fetch('/api/backups/restore', {
		method: 'POST',
		headers: { 'Content-Type': 'application/zip' },
		body: file
	});
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new ApiError(data.error ?? `HTTP ${res.status}`, res.status);
	return data as { restored: string; safetyBackup: string };
}

export const api = {
	organizePlan: (library: number, template: string, series = '') =>
		request<OrganizePlan>(
			'GET',
			`/api/libraries/${library}/organize?${new URLSearchParams({ template, series })}`
		),
	organizeApply: (library: number, template: string, editions: number[]) =>
		request<OrganizeOp>('POST', `/api/libraries/${library}/organize`, { template, editions }),
	organizeHistory: (library: number) => request<OrganizeOp[]>('GET', `/api/libraries/${library}/organize/history`),
	organizeUndo: (op: number) => request<OrganizeOp>('POST', `/api/organize/${op}/undo`),

	duplicates: () => request<DuplicatesReport>('GET', '/api/duplicates'),

	listBackups: () => request<BackupInfo[]>('GET', '/api/backups'),
	createBackup: () => request<BackupInfo>('POST', '/api/backups'),
	restoreBackup,
	backupUrl: (name: string) => `/api/backups/${encodeURIComponent(name)}`,

	listTasks: (status: TaskStatus | 'all' = 'open', limit = 100) =>
		request<{ tasks: Task[]; open: number }>('GET', `/api/tasks?status=${status}&limit=${limit}`),
	retryTask: (id: number) => request<{ queued: boolean }>('POST', `/api/tasks/${id}/retry`),
	dismissTask: (id: number) => request<{ dismissed: boolean }>('POST', `/api/tasks/${id}/dismiss`),
	testNotification: () => request<{ sent: boolean }>('POST', '/api/notifications/test'),
	testComicVine: () => request<{ ok: boolean }>('POST', '/api/metadata/comicvine/test'),

	listSeries: (f: CatalogFilter) => request<Page<Series>>('GET', `/api/catalog/series?${query(f)}`),
	listFiles: (f: CatalogFilter & { series?: string }) =>
		request<Page<CatalogFile>>('GET', `/api/catalog/files?${query(f)}`),
	publishers: (library?: number) =>
		request<string[]>('GET', `/api/catalog/publishers${library ? `?library=${library}` : ''}`),
	getFile: (id: number) =>
		request<{
			file: CatalogFile;
			path: string;
			readers: { kavita?: string; bookOrbit?: string };
			edition: EditionMeta | null;
		}>('GET', `/api/files/${id}`),
	identifyFile: (id: number) => request<{ queued: boolean }>('POST', `/api/files/${id}/identify`),
	fileCandidates: (id: number, q: { series: string; number: string; year: string }) =>
		request<{ candidates: Candidate[]; errors: string[] }>(
			'GET',
			`/api/files/${id}/candidates?${new URLSearchParams(q)}`
		),
	applyMetadata: (id: number, c: { provider: string; issueId: string; score: number }) =>
		request<EditionMeta>('POST', `/api/files/${id}/metadata`, c),
	identifyLibrary: (id: number) => request<{ queued: number }>('POST', `/api/libraries/${id}/identify`),
	identifySeries: (library: number, series: string) =>
		request<{ queued: number }>(
			'POST',
			`/api/catalog/series/identify?${new URLSearchParams({ library: String(library), series })}`
		),
	convertFile: (id: number) => request<{ queued: boolean }>('POST', `/api/files/${id}/convert`),

	listJobs: (status: JobFilter = 'active', limit = 100) =>
		request<{ jobs: Job[]; counts: Record<JobStatus, number>; turbo: Turbo }>(
			'GET',
			`/api/jobs?status=${status}&limit=${limit}`
		),
	retryJob: (id: number) => request<{ queued: boolean }>('POST', `/api/jobs/${id}/retry`),
	cancelJob: (id: number) => request<{ cancelled: boolean }>('POST', `/api/jobs/${id}/cancel`),
	convertLibrary: (id: number) => request<{ queued: number }>('POST', `/api/libraries/${id}/convert`),
	setTurbo: (enabled: boolean) => request<Turbo>('PUT', '/api/turbo', { enabled }),

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
