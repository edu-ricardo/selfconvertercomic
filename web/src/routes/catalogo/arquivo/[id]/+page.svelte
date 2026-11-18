<script lang="ts">
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import Cover from '$lib/components/cover.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { api, type CatalogFile } from '$lib/api';
	import { fileStatusLabel, fileStatusVariant, formatLabel, megabytes } from '$lib/status';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import FileArchiveIcon from '@lucide/svelte/icons/file-archive';

	const id = $derived(Number(page.params.id));

	let file = $state<CatalogFile | null>(null);
	let path = $state('');
	let readers = $state<{ kavita?: string; bookOrbit?: string }>({});
	let error = $state('');

	async function load(fileId: number) {
		try {
			const r = await api.getFile(fileId);
			file = r.file;
			path = r.path;
			readers = r.readers;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		load(id);
	});

	async function convert() {
		try {
			await api.convertFile(id);
			toast.success('Na fila de conversão');
			await load(id);
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		}
	}

	const seriesHref = $derived(
		file ? `/catalogo/serie?library=${file.libraryId}&key=${encodeURIComponent(file.series)}` : '/catalogo'
	);
	const modified = $derived(
		file ? new Date(file.mtime / 1e6).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : ''
	);
</script>

{#if error}
	<p class="text-destructive">Não foi possível carregar: {error}</p>
{:else if !file}
	<p class="text-muted-foreground">Carregando…</p>
{:else}
	<a href={seriesHref} class="text-muted-foreground hover:text-foreground mb-4 inline-flex items-center gap-1 text-sm">
		<ArrowLeftIcon class="size-4" /> {file.seriesName}
	</a>

	<div class="grid gap-6 md:grid-cols-[16rem_1fr]">
		<Cover id={file.id} mtime={file.mtime} alt="Capa de {file.title}" class="w-48 md:w-64" />

		<div class="grid content-start gap-4">
			<div>
				<h1 class="text-2xl font-semibold tracking-tight">{file.title}</h1>
				<p class="text-muted-foreground text-sm">
					{[file.publisher, file.seriesName].filter(Boolean).join(' · ')}
				</p>
			</div>
			<div class="flex flex-wrap gap-2">
				<Badge variant="outline">{formatLabel[file.format]}</Badge>
				<Badge variant={fileStatusVariant(file.status)}>{fileStatusLabel[file.status]}</Badge>
			</div>

			<div class="flex flex-wrap gap-2">
				{#if file.format === 'pdf' && (file.status === 'pending' || file.status === 'failed')}
					<Button onclick={convert}><FileArchiveIcon /> Converter para CBZ</Button>
				{/if}
				{#if readers.kavita}
					<Button variant="outline" href={readers.kavita} target="_blank" rel="noopener">
						<ExternalLinkIcon /> Abrir no Kavita
					</Button>
				{/if}
				{#if readers.bookOrbit}
					<Button variant="outline" href={readers.bookOrbit} target="_blank" rel="noopener">
						<ExternalLinkIcon /> Abrir no BookOrbit
					</Button>
				{/if}
			</div>
			{#if !readers.kavita && !readers.bookOrbit}
				<p class="text-muted-foreground text-xs">
					Configure o endereço do Kavita ou do BookOrbit em <a href="/configuracoes" class="underline">Configurações</a>
					para abrir os quadrinhos a partir daqui.
				</p>
			{/if}

			<Card.Root>
				<Card.Content class="grid gap-2 text-sm">
					<div class="grid grid-cols-[7rem_1fr] gap-2">
						<span class="text-muted-foreground">Biblioteca</span><span>{file.libraryName}</span>
						<span class="text-muted-foreground">Tamanho</span><span>{megabytes(file.size)}</span>
						<span class="text-muted-foreground">Modificado</span><span>{modified}</span>
						<span class="text-muted-foreground">Caminho</span>
						<span class="font-mono text-xs break-all">{path}</span>
					</div>
				</Card.Content>
			</Card.Root>
		</div>
	</div>
{/if}
