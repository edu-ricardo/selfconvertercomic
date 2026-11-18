<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import FolderPicker from '$lib/components/folder-picker.svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { api, type Library } from '$lib/api';
	import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

	let libraries = $state<Library[] | null>(null);
	let watchError = $state('');
	let loadError = $state('');

	let addOpen = $state(false);
	let newPath = $state('');
	let newName = $state('');
	let adding = $state(false);

	let toDelete = $state<Library | null>(null);

	async function load() {
		try {
			const r = await api.listLibraries();
			libraries = r.libraries;
			watchError = r.watchError;
			loadError = '';
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		}
	}

	// Atualiza sozinho enquanto houver varredura em andamento.
	$effect(() => {
		load();
		const t = setInterval(() => {
			if (libraries?.some((l) => l.scanning)) load();
		}, 2000);
		return () => clearInterval(t);
	});

	async function add(event: SubmitEvent) {
		event.preventDefault();
		adding = true;
		try {
			await api.createLibrary({ name: newName, path: newPath, autoConvert: true });
			toast.success('Biblioteca adicionada; varredura iniciada');
			addOpen = false;
			newName = '';
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao adicionar');
		} finally {
			adding = false;
		}
	}

	async function toggleAuto(l: Library, autoConvert: boolean) {
		try {
			await api.updateLibrary(l.id, { name: l.name, autoConvert });
			l.autoConvert = autoConvert;
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao salvar');
			await load();
		}
	}

	async function scanNow(l: Library) {
		await api.scanLibrary(l.id);
		l.scanning = true;
		toast.info(`Varrendo ${l.name}…`);
	}

	async function remove() {
		if (!toDelete) return;
		try {
			await api.deleteLibrary(toDelete.id);
			toast.success('Biblioteca removida do catálogo');
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao remover');
		} finally {
			toDelete = null;
		}
	}

	const fmtDate = (iso: string | null) =>
		iso ? new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : 'nunca';
</script>

<PageHeader title="Bibliotecas" description="Pastas que a QuadrinhoTeca acompanha, em qualquer disco.">
	{#snippet actions()}
		<Button onclick={() => (addOpen = true)}><PlusIcon /> Adicionar</Button>
	{/snippet}
</PageHeader>

{#if watchError}
	<div class="mb-4 flex gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
		<TriangleAlertIcon class="size-4 shrink-0 text-amber-600" />
		<span>{watchError}</span>
	</div>
{/if}

{#if loadError}
	<p class="text-destructive">Não foi possível carregar: {loadError}</p>
{:else if !libraries}
	<p class="text-muted-foreground">Carregando…</p>
{:else if libraries.length === 0}
	<EmptyState
		icon={FolderOpenIcon}
		title="Nenhuma biblioteca"
		text="Adicione a pasta onde estão seus quadrinhos (ex.: um HD externo)."
	/>
{:else}
	<div class="grid gap-4 lg:grid-cols-2">
		{#each libraries as l (l.id)}
			<Card.Root>
				<Card.Header>
					<Card.Title class="flex flex-wrap items-center gap-2">
						{l.name}
						{#if l.scanning}
							<Badge variant="secondary">Varrendo…</Badge>
						{:else if l.available}
							<Badge variant="outline">Disponível</Badge>
						{:else}
							<Badge variant="destructive">Indisponível</Badge>
						{/if}
					</Card.Title>
					<Card.Description class="truncate font-mono text-xs" title={l.path}>{l.path}</Card.Description>
				</Card.Header>
				<Card.Content class="grid gap-3 text-sm">
					<div class="flex gap-4">
						<span><strong>{l.counts.pdf}</strong> PDF</span>
						<span><strong>{l.counts.cbz}</strong> CBZ</span>
						<span><strong>{l.counts.cbr}</strong> CBR</span>
					</div>
					<p class="text-muted-foreground text-xs">Última varredura: {fmtDate(l.lastScanAt)}</p>
					{#if l.lastError}<p class="text-destructive text-xs">{l.lastError}</p>{/if}
					<div class="flex items-center justify-between gap-4">
						<Label for="auto-{l.id}">Converter PDFs novos automaticamente</Label>
						<Switch
							id="auto-{l.id}"
							checked={l.autoConvert}
							onCheckedChange={(v) => toggleAuto(l, v)}
						/>
					</div>
				</Card.Content>
				<Card.Footer class="flex justify-end gap-2">
					<Button variant="outline" size="sm" disabled={l.scanning} onclick={() => scanNow(l)}>
						<RefreshCwIcon /> Varrer agora
					</Button>
					<Button variant="ghost" size="sm" onclick={() => (toDelete = l)}>
						<Trash2Icon /> Remover
					</Button>
				</Card.Footer>
			</Card.Root>
		{/each}
	</div>
{/if}

<Dialog.Root bind:open={addOpen}>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Adicionar biblioteca</Dialog.Title>
			<Dialog.Description>Navegue até a pasta dos quadrinhos e confirme.</Dialog.Description>
		</Dialog.Header>
		<form class="grid gap-4" onsubmit={add}>
			{#if addOpen}<FolderPicker bind:value={newPath} />{/if}
			<div class="grid gap-1.5">
				<Label for="libName">Nome</Label>
				<Input id="libName" placeholder="Nome da pasta, se vazio" bind:value={newName} />
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (addOpen = false)}>Cancelar</Button>
				<Button type="submit" disabled={!newPath || adding}>
					{adding ? 'Adicionando…' : 'Usar esta pasta'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<AlertDialog.Root open={toDelete !== null} onOpenChange={(o) => !o && (toDelete = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Remover “{toDelete?.name}”?</AlertDialog.Title>
			<AlertDialog.Description>
				Sai só do catálogo da QuadrinhoTeca. Nenhum arquivo do disco é apagado.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancelar</AlertDialog.Cancel>
			<AlertDialog.Action onclick={remove}>Remover</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
