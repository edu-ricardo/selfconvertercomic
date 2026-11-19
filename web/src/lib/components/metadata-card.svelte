<script lang="ts">
	import { toast } from 'svelte-sonner';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { api, type Candidate, type EditionMeta } from '$lib/api';
	import { metaStatusLabel, providerLabel } from '$lib/status';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import SearchIcon from '@lucide/svelte/icons/search';
	import CheckIcon from '@lucide/svelte/icons/check';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';

	let {
		fileId,
		edition,
		onchange
	}: { fileId: number; edition: EditionMeta; onchange: () => Promise<void> } = $props();

	let picking = $state(false);
	let busy = $state(false);
	let searching = $state(false);
	let found = $state<Candidate[] | null>(null);
	let searchErrors = $state<string[]>([]);
	let q = $state({ series: '', number: '', year: '' });

	// Abre o seletor quando há tarefa de confirmação; o formulário começa com o que foi identificado.
	$effect(() => {
		if (edition.status === 'task') picking = true;
		q = {
			series: edition.guess.series,
			number: edition.guess.number,
			year: edition.guess.year ? String(edition.guess.year) : ''
		};
	});

	const candidates = $derived(found ?? edition.candidates);
	const waiting = $derived(edition.status === 'queued' || edition.status === 'rewrite');

	// Enquanto está na fila, recarrega até sair dela.
	$effect(() => {
		if (!waiting) return;
		const t = setInterval(onchange, 3000);
		return () => clearInterval(t);
	});

	async function identify() {
		busy = true;
		try {
			await api.identifyFile(fileId);
			toast.info('Na fila de identificação');
			await onchange();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		} finally {
			busy = false;
		}
	}

	async function search(event: SubmitEvent) {
		event.preventDefault();
		searching = true;
		try {
			const r = await api.fileCandidates(fileId, { series: q.series, number: q.number, year: q.year });
			found = r.candidates;
			searchErrors = r.errors;
			if (r.candidates.length === 0) toast.info('Nada encontrado com essa busca');
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha na busca');
		} finally {
			searching = false;
		}
	}

	async function use(c: Candidate) {
		busy = true;
		try {
			await api.applyMetadata(fileId, { provider: c.provider, issueId: c.issueId, score: c.score });
			toast.success('Metadados aplicados');
			picking = false;
			found = null;
			await onchange();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao aplicar');
		} finally {
			busy = false;
		}
	}

	const d = $derived(edition.applied);
	const people = (names: string[] | null) => (names ?? []).join(', ');
	const credits = $derived(
		d
			? [
					['Roteiro', people(d.writers)],
					['Desenho', people(d.pencillers)],
					['Arte-final', people(d.inkers)],
					['Cores', people(d.colorists)],
					['Letras', people(d.letterers)],
					['Capa', people(d.coverArtists)]
				].filter(([, v]) => v)
			: []
	);
</script>

<Card.Root>
	<Card.Header>
		<Card.Title class="flex flex-wrap items-center gap-2">
			Metadados
			<Badge variant={edition.status === 'task' || edition.status === 'error' ? 'destructive' : 'secondary'}>
				{metaStatusLabel[edition.status] ?? edition.status}
			</Badge>
		</Card.Title>
		{#if edition.status === 'error'}
			<Card.Description class="text-destructive">{edition.error}</Card.Description>
		{:else if edition.status === 'task'}
			<Card.Description>
				As fontes não deram certeza suficiente. Escolha a edição certa abaixo ou busque com outro nome.
			</Card.Description>
		{/if}
	</Card.Header>

	<Card.Content class="grid gap-4 text-sm">
		{#if d}
			<div class="grid grid-cols-[7rem_1fr] gap-2">
				<span class="text-muted-foreground">Série</span><span>{d.series} #{d.number}</span>
				{#if d.title}<span class="text-muted-foreground">Título</span><span>{d.title}</span>{/if}
				{#if d.year}
					<span class="text-muted-foreground">Data</span>
					<span>{d.month ? `${String(d.month).padStart(2, '0')}/` : ''}{d.year}</span>
				{/if}
				{#if d.publisher}<span class="text-muted-foreground">Editora</span><span>{d.publisher}</span>{/if}
				{#each credits as [label, value] (label)}
					<span class="text-muted-foreground">{label}</span><span>{value}</span>
				{/each}
				<span class="text-muted-foreground">Fonte</span>
				<span>
					<a href={d.url} target="_blank" rel="noopener" class="inline-flex items-center gap-1 underline">
						{providerLabel(d.provider)} <ExternalLinkIcon class="size-3" />
					</a>
					<span class="text-muted-foreground">· nota {edition.score}</span>
					{#if d.provider === 'gcd'}
						<span class="text-muted-foreground block text-xs">Dados do Grand Comics Database, licença CC BY-SA 4.0.</span>
					{/if}
				</span>
			</div>
			{#if d.summary}
				<p class="text-muted-foreground whitespace-pre-line">{d.summary}</p>
			{/if}
		{/if}

		{#if picking}
			<form class="flex flex-wrap items-end gap-2" onsubmit={search}>
				<div class="grid min-w-48 flex-1 gap-1.5">
					<Label for="q-series">Série</Label>
					<Input id="q-series" bind:value={q.series} />
				</div>
				<div class="grid w-20 gap-1.5">
					<Label for="q-number">Número</Label>
					<Input id="q-number" bind:value={q.number} />
				</div>
				<div class="grid w-24 gap-1.5">
					<Label for="q-year">Ano</Label>
					<Input id="q-year" inputmode="numeric" bind:value={q.year} />
				</div>
				<Button type="submit" variant="outline" disabled={searching || !q.series.trim()}>
					<SearchIcon /> {searching ? 'Buscando…' : 'Buscar'}
				</Button>
			</form>
			{#each searchErrors as err (err)}
				<p class="text-destructive text-xs">{err}</p>
			{/each}

			{#if candidates.length === 0}
				<p class="text-muted-foreground">Nenhum candidato. Tente outro nome (o original em inglês, por exemplo).</p>
			{:else}
				<ul class="grid gap-3 sm:grid-cols-2">
					{#each candidates as c (c.provider + c.seriesId + c.issueId)}
						<li class="flex gap-3 rounded-md border p-2">
							{#if c.coverUrl}
								<img
									src={c.coverUrl}
									alt="Capa de {c.series} #{c.number}"
									loading="lazy"
									referrerpolicy="no-referrer"
									class="bg-muted h-24 w-16 shrink-0 rounded object-cover"
								/>
							{:else}
								<div class="bg-muted h-24 w-16 shrink-0 rounded"></div>
							{/if}
							<div class="grid min-w-0 content-start gap-1">
								<p class="font-medium leading-tight">
									{c.series}{c.number ? ` #${c.number}` : ''}
									{#if c.year}<span class="text-muted-foreground">({c.year})</span>{/if}
								</p>
								{#if c.title}<p class="text-muted-foreground truncate text-xs">{c.title}</p>{/if}
								<p class="text-muted-foreground text-xs">
									{[providerLabel(c.provider), c.publisher, `nota ${c.score}`].filter(Boolean).join(' · ')}
								</p>
								<div class="flex flex-wrap gap-1">
									<Button size="sm" disabled={busy || !c.issueId} onclick={() => use(c)}>
										<CheckIcon /> Usar este
									</Button>
									<Button size="sm" variant="ghost" href={c.url} target="_blank" rel="noopener">
										<ExternalLinkIcon /> Ver
									</Button>
								</div>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</Card.Content>

	<Card.Footer class="flex flex-wrap gap-2">
		<Button type="button" variant="outline" size="sm" disabled={busy || waiting} onclick={identify}>
			<SparklesIcon /> {edition.status === 'applied' ? 'Identificar de novo' : 'Identificar'}
		</Button>
		{#if !picking}
			<Button type="button" variant="ghost" size="sm" onclick={() => (picking = true)}>
				<SearchIcon /> Escolher manualmente
			</Button>
		{/if}
	</Card.Footer>
</Card.Root>
