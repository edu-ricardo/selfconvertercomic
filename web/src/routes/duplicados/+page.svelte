<script lang="ts">
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { api, type DuplicatesReport } from '$lib/api';
	import { formatLabel, megabytes } from '$lib/status';
	import CopyIcon from '@lucide/svelte/icons/copy';

	let report = $state<DuplicatesReport | null>(null);
	let error = $state('');
	let kind = $state<'all' | 'content' | 'edition'>('all');

	async function load() {
		try {
			report = await api.duplicates();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		load();
	});
	// Enquanto o hash dos arquivos está sendo calculado, atualiza de tempos em tempos.
	$effect(() => {
		if (!report?.pending) return;
		const t = setInterval(load, 5000);
		return () => clearInterval(t);
	});

	const groups = $derived((report?.groups ?? []).filter((g) => kind === 'all' || g.kind === kind));
	const counts = $derived({
		content: report?.groups.filter((g) => g.kind === 'content').length ?? 0,
		edition: report?.groups.filter((g) => g.kind === 'edition').length ?? 0
	});
	const folder = (rel: string) => rel.slice(0, Math.max(0, rel.lastIndexOf('/')));
	const name = (rel: string) => rel.slice(rel.lastIndexOf('/') + 1);
</script>

<PageHeader
	title="Duplicados"
	description="Arquivos repetidos na biblioteca. A QuadrinhoTeca só mostra: decida você o que manter e apague pelo app Arquivos do Umbrel."
/>

{#if error}
	<p class="text-destructive">Não foi possível carregar: {error}</p>
{:else if !report}
	<p class="text-muted-foreground">Carregando…</p>
{:else}
	{#if report.pending > 0}
		<p class="text-muted-foreground mb-4 text-sm">
			Calculando o conteúdo dos arquivos: faltam {report.pending} de {report.total}. A lista de conteúdo idêntico
			ainda pode crescer.
		</p>
	{/if}

	<div class="mb-4 flex flex-wrap gap-2 text-sm">
		{#each [['all', `Todos (${counts.content + counts.edition})`], ['content', `Conteúdo idêntico (${counts.content})`], ['edition', `Mesma edição (${counts.edition})`]] as [value, label] (value)}
			<button
				type="button"
				class="rounded-md border px-3 py-1 {kind === value ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'}"
				onclick={() => (kind = value as typeof kind)}
			>
				{label}
			</button>
		{/each}
	</div>

	{#if groups.length === 0}
		<EmptyState icon={CopyIcon} title="Nenhum duplicado" text="Nada repetido por aqui." />
	{:else}
		<div class="grid gap-3">
			{#each groups as g, i (g.kind + i)}
				<Card.Root>
					<Card.Header class="pb-2">
						<Card.Title class="flex flex-wrap items-center gap-2 text-base">
							{g.label}
							<Badge variant="outline">{g.kind === 'content' ? 'conteúdo idêntico' : 'mesma edição em lugares diferentes'}</Badge>
						</Card.Title>
					</Card.Header>
					<Card.Content>
						<ul class="divide-y rounded-md border text-sm">
							{#each g.files as f (f.id)}
								<li class="flex items-center justify-between gap-2 px-3 py-2">
									<a href="/catalogo/arquivo/{f.id}" class="grid min-w-0 hover:underline">
										<span class="truncate font-medium">{name(f.relPath)}</span>
										<span class="text-muted-foreground truncate text-xs">{f.libraryName}{folder(f.relPath) ? ' / ' + folder(f.relPath) : ''}</span>
									</a>
									<span class="text-muted-foreground shrink-0 text-xs">{formatLabel[f.format]} · {megabytes(f.size)}</span>
								</li>
							{/each}
						</ul>
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
		{#if report.more}
			<p class="text-muted-foreground mt-4 text-xs">Mostrando os primeiros 200 grupos.</p>
		{/if}
	{/if}
{/if}
