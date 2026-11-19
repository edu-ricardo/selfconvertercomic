import type { CatalogFile, FileStatus, Format, MetaStatus } from './api';

export const fileStatusLabel: Record<FileStatus, string> = {
	pending: 'Não convertido',
	converting: 'Convertendo',
	failed: 'Erro na conversão',
	converted: 'Convertido',
	ready: 'Pronto'
};

export const fileStatusVariant = (s: FileStatus) =>
	s === 'failed' ? 'destructive' : s === 'converting' ? 'default' : s === 'pending' ? 'outline' : 'secondary';

export const formatLabel: Record<Format, string> = { pdf: 'PDF', cbz: 'CBZ', cbr: 'CBR' };

/** Volume e número da edição: "v3 #18", "#5", "" quando não se sabe. */
export const editionLabel = (f: Pick<CatalogFile, 'volume' | 'number'>) =>
	[f.volume && `v${f.volume}`, f.number && `#${f.number}`].filter(Boolean).join(' ');

export const metaStatusLabel: Record<MetaStatus, string> = {
	'': 'Não identificado',
	new: 'Aguardando identificação',
	queued: 'Na fila de identificação',
	applied: 'Identificado',
	rewrite: 'Gravando no CBZ…',
	task: 'Precisa de confirmação',
	error: 'Erro na identificação'
};

export const providerLabel = (p: string) =>
	p === 'gcd' ? 'GCD' : p === 'comicvine' ? 'ComicVine' : p === 'manual' ? 'preenchido à mão' : p;

export const megabytes = (n: number) =>
	(n / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + ' MB';
