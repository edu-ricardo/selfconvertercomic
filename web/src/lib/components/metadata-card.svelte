<script lang="ts">
	import { toast } from 'svelte-sonner';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { api, type ApplyBody, type Candidate, type EditionMeta } from '$lib/api';
	import LinkIcon from '@lucide/svelte/icons/link';
	import PencilIcon from '@lucide/svelte/icons/pencil';
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

	// Aplica e recarrega; as variantes só mudam o que é enviado.
	async function applyWith(body: ApplyBody, okMessage = 'Metadados aplicados') {
		busy = true;
		try {
			await api.applyMetadata(fileId, body);
			toast.success(okMessage);
			picking = false;
			editing = false;
			found = null;
			link = '';
			await onchange();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao aplicar');
		} finally {
			busy = false;
		}
	}

	const use = (c: Candidate) => applyWith({ provider: c.provider, issueId: c.issueId, score: c.score });

	// Candidato que é só a série: número digitado, ou o do arquivo; sem nenhum (encadernado, edição
	// especial), o servidor usa a edição única da série.
	let seriesNumber = $state<Record<string, string>>({});
	const seriesNumberOf = (c: Candidate) => (seriesNumber[c.provider + c.seriesId] || edition.guess.number || '').trim();
	const useSeries = (c: Candidate) =>
		applyWith({ provider: c.provider, seriesId: c.seriesId, number: seriesNumberOf(c) });

	// Link da página da edição (ou da série + número) no ComicVine ou no GCD.
	let link = $state('');
	let linkNumber = $state('');
	const useLink = (event: SubmitEvent) => {
		event.preventDefault();
		applyWith({ url: link.trim(), number: linkNumber.trim() });
	};

	// Preenchimento manual: começa com o que já se sabe.
	let editing = $state(false);
	let manual = $state(emptyManual());
	function emptyManual() {
		return {
			series: '', number: '', title: '', year: '', month: '', publisher: '', language: '',
			writers: '', pencillers: '', inkers: '', colorists: '', letterers: '', coverArtists: '', summary: ''
		};
	}
	function startEditing() {
		const a = edition.applied;
		const list = (l: string[] | null | undefined) => (l ?? []).join(', ');
		manual = {
			series: a?.series ?? edition.guess.series,
			number: a?.number ?? edition.guess.number,
			title: a?.title ?? '',
			year: String(a?.year || edition.guess.year || ''),
			month: a?.month ? String(a.month) : '',
			publisher: a?.publisher ?? '',
			language: a?.language ?? '',
			writers: list(a?.writers), pencillers: list(a?.pencillers), inkers: list(a?.inkers),
			colorists: list(a?.colorists), letterers: list(a?.letterers), coverArtists: list(a?.coverArtists),
			summary: a?.summary ?? ''
		};
		editing = true;
	}
	async function saveManual(event: SubmitEvent) {
		event.preventDefault();
		const names = (s: string) => s.split(',').map((n) => n.trim()).filter(Boolean);
		busy = true;
		try {
			await api.manualMetadata(fileId, {
				series: manual.series, number: manual.number, title: manual.title,
				year: Number(manual.year) || 0, month: Number(manual.month) || 0,
				publisher: manual.publisher, language: manual.language, summary: manual.summary,
				writers: names(manual.writers), pencillers: names(manual.pencillers), inkers: names(manual.inkers),
				colorists: names(manual.colorists), letterers: names(manual.letterers), coverArtists: names(manual.coverArtists)
			});
			toast.success('Metadados salvos');
			editing = false;
			picking = false;
			await onchange();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao salvar');
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
					{#if d.provider === 'manual'}
						Preenchido à mão
					{:else}
						<a href={d.url} target="_blank" rel="noopener" class="inline-flex items-center gap-1 underline">
							{providerLabel(d.provider)} <ExternalLinkIcon class="size-3" />
						</a>
						<span class="text-muted-foreground">· nota {edition.score}</span>
					{/if}
					{#if d.provider === 'gcd'}
						<span class="text-muted-foreground block text-xs">Dados do Grand Comics Database, licença CC BY-SA 4.0.</span>
					{/if}
				</span>
			</div>
			{#if d.summary}
				<p class="text-muted-foreground whitespace-pre-line">{d.summary}</p>
			{/if}
		{/if}

		{#if editing}
			<form class="grid gap-3" onsubmit={saveManual}>
				<div class="flex flex-wrap gap-2">
					<div class="grid min-w-48 flex-1 gap-1.5"><Label for="m-series">Série *</Label><Input id="m-series" bind:value={manual.series} /></div>
					<div class="grid w-20 gap-1.5"><Label for="m-number">Número</Label><Input id="m-number" bind:value={manual.number} /></div>
					<div class="grid w-24 gap-1.5"><Label for="m-year">Ano</Label><Input id="m-year" inputmode="numeric" bind:value={manual.year} /></div>
					<div class="grid w-16 gap-1.5"><Label for="m-month">Mês</Label><Input id="m-month" inputmode="numeric" bind:value={manual.month} /></div>
				</div>
				<div class="flex flex-wrap gap-2">
					<div class="grid min-w-48 flex-1 gap-1.5"><Label for="m-title">Título</Label><Input id="m-title" bind:value={manual.title} /></div>
					<div class="grid min-w-40 flex-1 gap-1.5"><Label for="m-pub">Editora</Label><Input id="m-pub" bind:value={manual.publisher} /></div>
					<div class="grid w-20 gap-1.5"><Label for="m-lang">Idioma</Label><Input id="m-lang" placeholder="pt" bind:value={manual.language} /></div>
				</div>
				<div class="grid gap-2 sm:grid-cols-2">
					{#each [['writers', 'Roteiro'], ['pencillers', 'Desenho'], ['inkers', 'Arte-final'], ['colorists', 'Cores'], ['letterers', 'Letras'], ['coverArtists', 'Capa']] as [key, label] (key)}
						<div class="grid gap-1.5">
							<Label for="m-{key}">{label}</Label>
							<Input id="m-{key}" placeholder="nomes separados por vírgula" bind:value={manual[key as keyof typeof manual]} />
						</div>
					{/each}
				</div>
				<div class="grid gap-1.5">
					<Label for="m-summary">Sinopse</Label>
					<textarea id="m-summary" rows="4" class="border-input bg-background rounded-md border px-3 py-2 text-sm" bind:value={manual.summary}></textarea>
				</div>
				<div class="flex gap-2">
					<Button type="submit" disabled={busy || !manual.series.trim()}><CheckIcon /> Salvar</Button>
					<Button type="button" variant="ghost" onclick={() => (editing = false)}>Cancelar</Button>
				</div>
			</form>
		{/if}

		{#if picking}
			<form class="flex flex-wrap items-end gap-2" onsubmit={useLink}>
				<div class="grid min-w-48 flex-1 gap-1.5">
					<Label for="q-link">Link da edição no ComicVine ou no GCD</Label>
					<Input id="q-link" placeholder="https://www.comics.org/issue/… ou https://comicvine.gamespot.com/…/4000-…/" bind:value={link} />
				</div>
				<div class="grid w-20 gap-1.5">
					<Label for="q-link-number">Número</Label>
					<Input id="q-link-number" placeholder="se série" bind:value={linkNumber} />
				</div>
				<Button type="submit" variant="outline" disabled={busy || !link.trim()}>
					<LinkIcon /> Usar link
				</Button>
			</form>

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
								<div class="flex flex-wrap items-center gap-1">
									{#if c.issueId}
										<Button size="sm" disabled={busy} onclick={() => use(c)}>
											<CheckIcon /> Usar este
										</Button>
									{:else}
										<!-- Só a série (o arquivo não tem número): escolha qual edição dela. -->
										<Input
											class="h-8 w-16"
											aria-label="Número da edição em {c.series}"
											placeholder="nº"
											bind:value={seriesNumber[c.provider + c.seriesId]}
										/>
										<Button size="sm" disabled={busy} onclick={() => useSeries(c)}>
											<CheckIcon /> {seriesNumberOf(c) ? `Usar #${seriesNumberOf(c)}` : 'Usar edição única'}
										</Button>
									{/if}
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
				<SearchIcon /> Buscar ou colar link
			</Button>
		{/if}
		{#if !editing}
			<Button type="button" variant="ghost" size="sm" onclick={startEditing}>
				<PencilIcon /> Preencher à mão
			</Button>
		{/if}
	</Card.Footer>
</Card.Root>
