import type { CatalogFile, FileStatus, Format } from './api';

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

export const megabytes = (n: number) =>
	(n / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + ' MB';
