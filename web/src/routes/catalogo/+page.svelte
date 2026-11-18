<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import CatalogFilters from '$lib/components/catalog-filters.svelte';
	import Cover from '$lib/components/cover.svelte';
	import { Button } from '$lib/components/ui/button';
	import { api, type CatalogFilter, type Series } from '$lib/api';
	import { filterFromURL, filterToQuery } from '$lib/catalog-url';
	import LibraryIcon from '@lucide/svelte/icons/library-big';

	const PAGE = 60;
	const filter = $derived(filterFromURL(page.url.searchParams));

	let items = $state<Series[] | null>(null);
	let total = $state(0);
	let loading = $state(false);
	let error = $state('');
	let hasLibraries = $state(true);

	async function load(f: CatalogFilter, offset: number) {
		loading = true;
		try {
			const r = await api.listSeries({ ...f, offset, limit: PAGE });
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
		load(filter, 0);
	});
	$effect(() => {
		api.listLibraries().then((r) => (hasLibraries = r.libraries.length > 0)).catch(() => {});
	});

	function change(f: CatalogFilter) {
		goto(filterToQuery(f) || '/catalogo', { replaceState: true, keepFocus: true, noScroll: true });
	}

	const seriesHref = (s: Series) =>
		'/catalogo/serie' + filterToQuery({ ...filter, library: s.libraryId }, { key: s.key });
</script>

<PageHeader
	title="Catálogo"
	description={items ? `${total.toLocaleString('pt-BR')} série(s)` : 'Seus quadrinhos, agrupados por série.'}
/>

<CatalogFilters {filter} onchange={change} />

{#if error}
	<p class="text-destructive">Não foi possível carregar: {error}</p>
{:else if !items}
	<p class="text-muted-foreground">Carregando…</p>
{:else if items.length === 0}
	{#if hasLibraries}
		<EmptyState icon={LibraryIcon} title="Nada encontrado" text="Nenhuma série com esses filtros." />
	{:else}
		<EmptyState
			icon={LibraryIcon}
			title="Nenhuma biblioteca ainda"
			text="Cadastre a pasta dos seus quadrinhos em Bibliotecas."
		/>
	{/if}
{:else}
	<ul class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
		{#each items as s (s.libraryId + '/' + s.key)}
			<li>
				<a href={seriesHref(s)} class="group grid gap-2">
					<Cover id={s.coverFileId} alt="Capa de {s.name}" class="transition group-hover:ring-2 group-hover:ring-primary" />
					<div class="min-w-0">
						<p class="line-clamp-2 text-sm leading-tight font-medium" title={s.name}>{s.name}</p>
						<p class="text-muted-foreground truncate text-xs">
							{[s.publisher, `${s.count} ${s.count === 1 ? 'edição' : 'edições'}`].filter(Boolean).join(' · ')}
						</p>
						<p class="text-muted-foreground text-[11px]">
							{Object.entries(s.formats)
								.filter(([, n]) => n > 0)
								.map(([f, n]) => `${n} ${f.toUpperCase()}`)
								.join(' · ')}
						</p>
					</div>
				</a>
			</li>
		{/each}
	</ul>
	{#if items.length < total}
		<div class="mt-6 flex justify-center">
			<Button variant="outline" disabled={loading} onclick={() => load(filter, items?.length ?? 0)}>
				{loading ? 'Carregando…' : `Carregar mais (${total - items.length})`}
			</Button>
		</div>
	{/if}
{/if}
