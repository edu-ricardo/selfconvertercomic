<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { api, type DuplicateFile, type DuplicatesReport, type TrashItem } from '$lib/api';
	import { formatLabel, megabytes } from '$lib/status';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';

	let report = $state<DuplicatesReport | null>(null);
	let convertedPdfs = $state<DuplicateFile[]>([]);
	let trash = $state<TrashItem[]>([]);
	let error = $state('');
	let kind = $state<'all' | 'content' | 'edition'>('all');
	let selected = $state(new Set<number>());
	let folder = $state('');
	let busy = $state(false);

	async function load() {
		try {
			[report, convertedPdfs, trash] = await Promise.all([api.duplicates(), api.convertedPdfs(), api.listTrash()]);
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
	const dirOf = (rel: string) => rel.slice(0, Math.max(0, rel.lastIndexOf('/')));
	const nameOf = (rel: string) => rel.slice(rel.lastIndexOf('/') + 1);
	const sizeOf = (files: { size: number }[]) => megabytes(files.reduce((n, f) => n + f.size, 0));

	// "Uma pasta inteira": pastas de 1 e 2 níveis onde há cópias idênticas; exclui só as cópias que têm
	// outra igual fora da pasta.
	const folders = $derived.by(() => {
		const set = new Set<string>();
		for (const g of report?.groups ?? []) {
			if (g.kind !== 'content') continue;
			for (const f of g.files) {
				const parts = f.relPath.split('/');
				if (parts.length > 1) set.add(parts[0]);
				if (parts.length > 2) set.add(parts.slice(0, 2).join('/'));
			}
		}
		return [...set].sort();
	});
	const inFolder = (rel: string) => folder !== '' && (rel === folder || rel.startsWith(folder + '/'));
	const folderFiles = $derived(
		(report?.groups ?? [])
			.filter((g) => g.kind === 'content' && g.files.some((f) => !inFolder(f.relPath)))
			.flatMap((g) => g.files.filter((f) => inFolder(f.relPath)))
	);

	function toggle(id: number) {
		const s = new Set(selected);
		if (s.has(id)) s.delete(id);
		else s.add(id);
		selected = s;
	}
	const selectedFiles = $derived(
		(report?.groups ?? []).flatMap((g) => g.files).filter((f, i, all) => selected.has(f.id) && all.findIndex((o) => o.id === f.id) === i)
	);

	// Confirmação: o que vai para a lixeira.
	let confirm = $state<{ title: string; files: DuplicateFile[]; reason: string } | null>(null);

	async function moveToTrash() {
		if (!confirm) return;
		const { files, reason } = confirm;
		confirm = null;
		busy = true;
		try {
			const { moved } = await api.moveToTrash(files.map((f) => f.id), reason);
			toast.success(`${moved} arquivo(s) na lixeira`);
			selected = new Set();
			folder = '';
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		} finally {
			busy = false;
		}
	}

	// Lixeira.
	let trashSelected = $state(new Set<number>());
	let confirmEmpty = $state(false);
	function toggleTrash(id: number) {
		const s = new Set(trashSelected);
		if (s.has(id)) s.delete(id);
		else s.add(id);
		trashSelected = s;
	}
	async function restore() {
		busy = true;
		try {
			const { restored } = await api.restoreTrash([...trashSelected]);
			toast.success(`${restored} arquivo(s) de volta ao lugar`);
			trashSelected = new Set();
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		} finally {
			busy = false;
		}
	}
	async function emptyTrash() {
		confirmEmpty = false;
		busy = true;
		try {
			const { deleted } = await api.emptyTrash();
			toast.success(`${deleted} arquivo(s) apagado(s) de vez`);
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		} finally {
			busy = false;
		}
	}
	const when = (iso: string) => new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' });
</script>

<PageHeader
	title="Duplicados"
	description="Arquivos repetidos na biblioteca. Nada é excluído sem você pedir; o que você excluir vai para a lixeira da app e pode voltar."
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

	<Card.Root class="mb-6">
		<Card.Header>
			<Card.Title>Excluir em lote</Card.Title>
			<Card.Description>Vai para a lixeira da app. Nunca exclui a última cópia de um quadrinho.</Card.Description>
		</Card.Header>
		<Card.Content class="grid gap-4 text-sm">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span>
					<strong>PDFs que já têm CBZ</strong>
					<span class="text-muted-foreground">· {convertedPdfs.length} arquivo(s), {sizeOf(convertedPdfs)}</span>
				</span>
				<Button
					size="sm"
					variant="outline"
					disabled={busy || convertedPdfs.length === 0}
					onclick={() => (confirm = { title: `Excluir ${convertedPdfs.length} PDF(s) já convertido(s)?`, files: convertedPdfs, reason: 'PDF já convertido' })}
				>
					<Trash2Icon /> Excluir PDFs
				</Button>
			</div>
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span class="flex flex-wrap items-center gap-2">
					<strong>Cópias de uma pasta</strong>
					<Select.Root type="single" value={folder} onValueChange={(v) => (folder = v)}>
						<Select.Trigger class="h-8 w-64">{folder || 'Escolha a pasta'}</Select.Trigger>
						<Select.Content>
							{#each folders as f (f)}
								<Select.Item value={f}>{f}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
					{#if folder}<span class="text-muted-foreground">· {folderFiles.length} cópia(s), {sizeOf(folderFiles)}</span>{/if}
				</span>
				<Button
					size="sm"
					variant="outline"
					disabled={busy || folderFiles.length === 0}
					onclick={() => (confirm = { title: `Excluir ${folderFiles.length} cópia(s) que estão em ${folder}?`, files: folderFiles, reason: `cópia em ${folder}` })}
				>
					<Trash2Icon /> Excluir cópias
				</Button>
			</div>
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span>
					<strong>Escolhidos um a um</strong>
					<span class="text-muted-foreground">· marque abaixo · {selectedFiles.length} arquivo(s)</span>
				</span>
				<Button
					size="sm"
					variant="outline"
					disabled={busy || selectedFiles.length === 0}
					onclick={() => (confirm = { title: `Excluir ${selectedFiles.length} arquivo(s) marcado(s)?`, files: selectedFiles, reason: 'escolhido nos duplicados' })}
				>
					<Trash2Icon /> Excluir marcados
				</Button>
			</div>
		</Card.Content>
	</Card.Root>

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
								<li class="flex items-center gap-3 px-3 py-2">
									<input type="checkbox" aria-label="Marcar {f.relPath}" checked={selected.has(f.id)} onchange={() => toggle(f.id)} />
									<a href="/catalogo/arquivo/{f.id}" class="grid min-w-0 flex-1 hover:underline">
										<span class="truncate font-medium">{nameOf(f.relPath)}</span>
										<span class="text-muted-foreground truncate text-xs">{f.libraryName}{dirOf(f.relPath) ? ' / ' + dirOf(f.relPath) : ''}</span>
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

	<Card.Root class="mt-6">
		<Card.Header>
			<Card.Title>Lixeira</Card.Title>
			<Card.Description>
				{trash.length} arquivo(s), {sizeOf(trash)}. Fica numa pasta oculta (.quadrinhoteca-lixeira) da própria
				biblioteca até você esvaziar.
			</Card.Description>
		</Card.Header>
		{#if trash.length > 0}
			<Card.Content>
				<ul class="divide-y rounded-md border text-sm">
					{#each trash as it (it.id)}
						<li class="flex items-center gap-3 px-3 py-2">
							<input type="checkbox" aria-label="Marcar {it.relPath}" checked={trashSelected.has(it.id)} onchange={() => toggleTrash(it.id)} />
							<span class="grid min-w-0 flex-1">
								<span class="truncate">{it.relPath}</span>
								<span class="text-muted-foreground text-xs">{it.libraryName} · {when(it.deletedAt)}{it.reason ? ' · ' + it.reason : ''}</span>
							</span>
							<span class="text-muted-foreground shrink-0 text-xs">{megabytes(it.size)}</span>
						</li>
					{/each}
				</ul>
			</Card.Content>
			<Card.Footer class="flex flex-wrap gap-2">
				<Button size="sm" variant="outline" disabled={busy || trashSelected.size === 0} onclick={restore}>
					<RotateCcwIcon /> Restaurar marcados ({trashSelected.size})
				</Button>
				<Button size="sm" variant="destructive" disabled={busy} onclick={() => (confirmEmpty = true)}>
					<Trash2Icon /> Esvaziar lixeira
				</Button>
			</Card.Footer>
		{/if}
	</Card.Root>
{/if}

<AlertDialog.Root open={confirm !== null} onOpenChange={(o) => !o && (confirm = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{confirm?.title}</AlertDialog.Title>
			<AlertDialog.Description>
				{confirm ? sizeOf(confirm.files) : ''} vão para a lixeira da app (dá para restaurar até esvaziar). Kavita e
				BookOrbit deixam de ver esses arquivos na próxima varredura deles.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancelar</AlertDialog.Cancel>
			<AlertDialog.Action onclick={moveToTrash}>Mover para a lixeira</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root open={confirmEmpty} onOpenChange={(o) => !o && (confirmEmpty = false)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Apagar de vez {trash.length} arquivo(s)?</AlertDialog.Title>
			<AlertDialog.Description>
				{sizeOf(trash)} serão apagados do disco. Isso não pode ser desfeito.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancelar</AlertDialog.Cancel>
			<AlertDialog.Action onclick={emptyTrash}>Apagar de vez</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
