<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import CatalogFilters from '$lib/components/catalog-filters.svelte';
	import Cover from '$lib/components/cover.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { api, type CatalogFile, type CatalogFilter } from '$lib/api';
	import { filterFromURL, filterToQuery } from '$lib/catalog-url';
	import { editionLabel, fileStatusLabel, fileStatusVariant } from '$lib/status';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import BookIcon from '@lucide/svelte/icons/book';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import { toast } from 'svelte-sonner';

	const PAGE = 60;
	const filter = $derived(filterFromURL(page.url.searchParams));
	const key = $derived(page.url.searchParams.get('key') ?? '');

	let items = $state<CatalogFile[] | null>(null);
	let total = $state(0);
	let loading = $state(false);
	let error = $state('');

	async function load(f: CatalogFilter, series: string, offset: number) {
		loading = true;
		try {
			const r = await api.listFiles({ ...f, series, offset, limit: PAGE });
			items = offset === 0 ? r.items : [...(items ?? []), ...r.items];
			total = r.total;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load(filter, key, 0);
	});

	const title = $derived(items?.[0]?.seriesName ?? (key.split('/').pop() || 'Série'));
	const subtitle = $derived(
		items?.[0] ? [items[0].libraryName, items[0].publisher].filter(Boolean).join(' · ') : ''
	);
	const back = $derived('/catalogo' + filterToQuery({ ...filter, library: undefined }));

	async function identifySeries() {
		const lib = items?.[0]?.libraryId;
		if (!lib) return;
		try {
			const { queued } = await api.identifySeries(lib, key);
			toast.success(queued ? `${queued} edição(ões) na fila de identificação` : 'Todas as edições já têm metadados');
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		}
	}

	function change(f: CatalogFilter) {
		goto(filterToQuery({ ...f, library: filter.library }, { key }), {
			replaceState: true,
			keepFocus: true,
			noScroll: true
		});
	}
</script>

<a href={back} class="text-muted-foreground hover:text-foreground mb-2 inline-flex items-center gap-1 text-sm">
	<ArrowLeftIcon class="size-4" /> Catálogo
</a>
<PageHeader {title} description={subtitle ? `${subtitle} · ${total} arquivo(s)` : undefined}>
	{#snippet actions()}
		<Button variant="outline" size="sm" disabled={!items?.length} onclick={identifySeries}>
			<SparklesIcon /> Identificar série
		</Button>
	{/snippet}
</PageHeader>

<CatalogFilters {filter} onchange={change} showLibrary={false} />

{#if error}
	<p class="text-destructive">Não foi possível carregar: {error}</p>
{:else if !items}
	<p class="text-muted-foreground">Carregando…</p>
{:else if items.length === 0}
	<EmptyState icon={BookIcon} title="Nada encontrado" text="Nenhum arquivo desta série com esses filtros." />
{:else}
	<ul class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
		{#each items as f (f.id)}
			<li>
				<a href="/catalogo/arquivo/{f.id}" class="group grid gap-2">
					<div class="relative">
						<Cover id={f.id} mtime={f.mtime} alt="Capa de {f.title}" class="transition group-hover:ring-2 group-hover:ring-primary" />
						<span class="absolute top-1.5 right-1.5 rounded bg-black/70 px-1.5 py-0.5 text-[10px] font-semibold text-white uppercase">
							{f.format}
						</span>
					</div>
					<div class="grid min-w-0 gap-1">
						<p class="line-clamp-2 text-sm leading-tight font-medium" title={f.title}>
							{#if f.number}
								<span
									class="text-muted-foreground"
									title={f.numberInferred ? 'Número deduzido pela ordem de leitura' : undefined}
								>{editionLabel(f)}</span>
							{/if}
							{f.title}
						</p>
						{#if f.status !== 'ready'}
							<Badge variant={fileStatusVariant(f.status)}>{fileStatusLabel[f.status]}</Badge>
						{/if}
					</div>
				</a>
			</li>
		{/each}
	</ul>
	{#if items.length < total}
		<div class="mt-6 flex justify-center">
			<Button variant="outline" disabled={loading} onclick={() => load(filter, key, items?.length ?? 0)}>
				{loading ? 'Carregando…' : `Carregar mais (${total - items.length})`}
			</Button>
		</div>
	{/if}
{/if}
