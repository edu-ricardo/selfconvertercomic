<script lang="ts">
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { api, ApiError, type Library, type OrganizeOp, type OrganizePlan } from '$lib/api';
	import FolderTreeIcon from '@lucide/svelte/icons/folder-tree';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import Undo2Icon from '@lucide/svelte/icons/undo-2';
	import SaveIcon from '@lucide/svelte/icons/save';

	// Mostra no máximo isso de linhas (o resto é aplicado igual, se marcado em bloco).
	const MAX_ROWS = 300;
	const variables = ['editora', 'serie', 'ano', 'numero', 'numero3', 'volume', 'titulo', 'original'];

	let libraries = $state<Library[]>([]);
	let libraryId = $state(Number(page.url.searchParams.get('library')) || 0);
	const series = page.url.searchParams.get('series') ?? '';
	let template = $state('');
	let savedTemplate = $state('');
	let plan = $state<OrganizePlan | null>(null);
	let planError = $state('');
	let loading = $state(false);
	let selected = $state(new Set<number>());
	let onlyProblems = $state(false);
	let confirming = $state(false);
	let applying = $state(false);
	let history = $state<OrganizeOp[]>([]);

	$effect(() => {
		Promise.all([api.listLibraries(), api.getSettings()])
			.then(([l, s]) => {
				libraries = l.libraries;
				if (!libraryId && libraries.length) libraryId = libraries[0].id;
				template = savedTemplate = s.organize.template;
			})
			.catch((e) => toast.error(e.message));
	});

	async function loadPlan(lib: number, tpl: string) {
		loading = true;
		try {
			plan = await api.organizePlan(lib, tpl, series);
			planError = '';
			selected = new Set(plan.items.filter((i) => !i.problem).map((i) => i.editionId));
		} catch (e) {
			plan = null;
			planError = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	async function loadHistory(lib: number) {
		try {
			history = await api.organizeHistory(lib);
		} catch {
			history = [];
		}
	}

	// Prévia ao digitar (com espera para não refazer o plano a cada tecla).
	$effect(() => {
		const lib = libraryId;
		const tpl = template;
		if (!lib || !tpl) return;
		const t = setTimeout(() => loadPlan(lib, tpl), 400);
		return () => clearTimeout(t);
	});
	$effect(() => {
		if (libraryId) loadHistory(libraryId);
	});

	const rows = $derived((plan?.items ?? []).filter((i) => !onlyProblems || i.problem).slice(0, MAX_ROWS));
	const ok = $derived((plan?.items ?? []).filter((i) => !i.problem));
	const problems = $derived((plan?.items.length ?? 0) - ok.length);

	function toggle(id: number) {
		const s = new Set(selected);
		if (s.has(id)) s.delete(id);
		else s.add(id);
		selected = s;
	}
	const allSelected = $derived(ok.length > 0 && ok.every((i) => selected.has(i.editionId)));
	function toggleAll() {
		selected = allSelected ? new Set() : new Set(ok.map((i) => i.editionId));
	}

	async function apply() {
		confirming = false;
		applying = true;
		try {
			const op = await api.organizeApply(libraryId, template, [...selected]);
			if (op.status === 'done') toast.success(`${op.editions} edição(ões) organizada(s) (${op.moves} arquivo(s))`);
			else toast.error(`Parou no meio: ${op.error}. O que já foi movido pode ser desfeito no histórico.`, { duration: 10000 });
			await Promise.all([loadPlan(libraryId, template), loadHistory(libraryId)]);
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao aplicar');
		} finally {
			applying = false;
		}
	}

	async function undo(op: OrganizeOp) {
		try {
			await api.organizeUndo(op.id);
			toast.success('Desfeito: os arquivos voltaram ao lugar de antes');
			await Promise.all([loadPlan(libraryId, template), loadHistory(libraryId)]);
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao desfazer');
		}
	}

	async function saveTemplate() {
		try {
			const s = await api.getSettings();
			s.organize.template = template;
			await api.putSettings(s);
			savedTemplate = template;
			toast.success('Padrão salvo');
		} catch (e) {
			toast.error(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : 'Falha ao salvar');
		}
	}

	const when = (iso: string) => new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' });
	const statusLabel: Record<string, string> = { done: 'Aplicada', partial: 'Parou no meio', undone: 'Desfeita' };
</script>

<PageHeader
	title="Organizar"
	description="Renomeia e move os quadrinhos para um padrão de pastas e nomes. Nada muda sem a sua aprovação, e tudo pode ser desfeito."
/>

{#if libraries.length === 0}
	<EmptyState icon={FolderTreeIcon} title="Nenhuma biblioteca" text="Cadastre uma biblioteca primeiro." />
{:else}
	<div class="grid gap-6">
		<Card.Root>
			<Card.Content class="grid gap-4">
				<div class="grid gap-4 sm:grid-cols-[14rem_1fr]">
					<div class="grid gap-1.5">
						<Label>Biblioteca</Label>
						<Select.Root
							type="single"
							value={String(libraryId)}
							onValueChange={(v) => (libraryId = Number(v))}
						>
							<Select.Trigger class="w-full">
								{libraries.find((l) => l.id === libraryId)?.name ?? 'Escolha'}
							</Select.Trigger>
							<Select.Content>
								{#each libraries as l (l.id)}
									<Select.Item value={String(l.id)}>{l.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<div class="grid gap-1.5">
						<Label for="tpl">Padrão de pastas e nome</Label>
						<div class="flex gap-2">
							<Input id="tpl" class="font-mono text-sm" bind:value={template} />
							<Button variant="outline" disabled={template === savedTemplate} onclick={saveTemplate}>
								<SaveIcon /> Salvar
							</Button>
						</div>
						<p class="text-muted-foreground text-xs">
							Variáveis: {#each variables as v, i (v)}<code>{`{${v}}`}</code>{i < variables.length - 1 ? ' ' : ''}{/each}.
							Use / para pastas. Partes sem valor somem; edições sem série ou número não são movidas.
						</p>
					</div>
				</div>
				{#if series}
					<p class="text-muted-foreground text-sm">Só a série escolhida no catálogo. <a href="/organizar?library={libraryId}" class="underline">Ver a biblioteca toda</a></p>
				{/if}
			</Card.Content>
		</Card.Root>

		{#if planError}
			<p class="text-destructive">{planError}</p>
		{:else if !plan}
			<p class="text-muted-foreground">{loading ? 'Calculando…' : ''}</p>
		{:else if plan.items.length === 0}
			<EmptyState icon={FolderTreeIcon} title="Tudo no lugar" text="{plan.unchanged} edição(ões) já seguem esse padrão." />
		{:else}
			<Card.Root>
				<Card.Header>
					<Card.Title>Prévia</Card.Title>
					<Card.Description>
						{ok.length} edição(ões) para mover · {problems} com problema (não serão movidas) · {plan.unchanged} já no lugar
					</Card.Description>
				</Card.Header>
				<Card.Content class="grid gap-3">
					<div class="flex flex-wrap items-center gap-4 text-sm">
						<label class="flex items-center gap-2">
							<input type="checkbox" checked={allSelected} onchange={toggleAll} disabled={ok.length === 0} />
							Marcar todas sem problema
						</label>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={onlyProblems} /> Só as com problema
						</label>
					</div>
					<ul class="divide-y rounded-md border text-sm">
						{#each rows as it (it.editionId)}
							<li class="grid grid-cols-[auto_1fr] gap-3 px-3 py-2" class:opacity-60={!!it.problem}>
								<input
									type="checkbox"
									class="mt-1"
									aria-label="Mover {it.from}"
									disabled={!!it.problem}
									checked={selected.has(it.editionId)}
									onchange={() => toggle(it.editionId)}
								/>
								<div class="grid min-w-0 gap-0.5">
									<span class="text-muted-foreground truncate" title={it.from}>{it.from}</span>
									<span class="flex min-w-0 items-center gap-1">
										<ArrowRightIcon class="size-3 shrink-0" />
										<span class="truncate font-medium" title={it.to}>{it.to}</span>
									</span>
									<span class="flex flex-wrap gap-1">
										<Badge variant="outline">{it.source === 'fontes' ? 'metadados das fontes' : 'pelo nome do arquivo'}</Badge>
										{#if it.files.length > 1}<Badge variant="outline">{it.files.length} arquivos</Badge>{/if}
										{#if it.problem}<Badge variant="destructive">{it.problem}</Badge>{/if}
									</span>
								</div>
							</li>
						{/each}
					</ul>
					{#if rows.length === MAX_ROWS}
						<p class="text-muted-foreground text-xs">Mostrando as primeiras {MAX_ROWS}.</p>
					{/if}
				</Card.Content>
				<Card.Footer>
					<Button disabled={selected.size === 0 || applying || loading} onclick={() => (confirming = true)}>
						<FolderTreeIcon /> {applying ? 'Aplicando…' : `Aplicar (${selected.size})`}
					</Button>
				</Card.Footer>
			</Card.Root>
		{/if}

		{#if history.length > 0}
			<Card.Root>
				<Card.Header><Card.Title>Histórico</Card.Title></Card.Header>
				<Card.Content>
					<ul class="divide-y rounded-md border text-sm">
						{#each history as op (op.id)}
							<li class="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
								<div class="grid min-w-0">
									<span>
										{when(op.createdAt)} · {op.editions} edição(ões), {op.moves} arquivo(s)
										<Badge variant={op.status === 'partial' ? 'destructive' : 'secondary'}>{statusLabel[op.status] ?? op.status}</Badge>
									</span>
									<span class="text-muted-foreground truncate font-mono text-xs" title={op.template}>{op.template}</span>
									{#if op.error}<span class="text-destructive text-xs">{op.error}</span>{/if}
								</div>
								{#if op.status !== 'undone' && op.moves > 0}
									<Button size="sm" variant="outline" onclick={() => undo(op)}><Undo2Icon /> Desfazer</Button>
								{/if}
							</li>
						{/each}
					</ul>
				</Card.Content>
			</Card.Root>
		{/if}
	</div>
{/if}

<AlertDialog.Root open={confirming} onOpenChange={(o) => !o && (confirming = false)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Mover {selected.size} edição(ões)?</AlertDialog.Title>
			<AlertDialog.Description>
				Os arquivos (PDF e CBZ de cada edição juntos) vão para os nomes da prévia. Kavita e BookOrbit vão
				ver os novos caminhos na próxima varredura deles. Dá para desfazer pelo histórico.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancelar</AlertDialog.Cancel>
			<AlertDialog.Action onclick={apply}>Mover</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
