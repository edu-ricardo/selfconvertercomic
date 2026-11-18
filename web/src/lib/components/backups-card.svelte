<script lang="ts">
	import { toast } from 'svelte-sonner';
	import * as Card from '$lib/components/ui/card';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { api, type BackupInfo } from '$lib/api';
	import { megabytes } from '$lib/status';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import DatabaseBackupIcon from '@lucide/svelte/icons/database-backup';
	import UploadIcon from '@lucide/svelte/icons/upload';

	let list = $state<BackupInfo[]>([]);
	let busy = $state(false);
	let pending = $state<File | null>(null);
	let input: HTMLInputElement;

	async function load() {
		try {
			list = await api.listBackups();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao listar backups');
		}
	}

	$effect(() => {
		load();
	});

	async function createNow() {
		busy = true;
		try {
			const b = await api.createBackup();
			toast.success(`Backup criado (${megabytes(b.size)})`);
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao criar backup');
		} finally {
			busy = false;
		}
	}

	async function restore() {
		if (!pending) return;
		const file = pending;
		pending = null;
		busy = true;
		try {
			const r = await api.restoreBackup(file);
			toast.success(`Backup restaurado. O estado anterior foi guardado em ${r.safetyBackup}.`, { duration: 10000 });
			setTimeout(() => location.reload(), 1500);
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha ao restaurar');
		} finally {
			busy = false;
			input.value = '';
			await load();
		}
	}

	const when = (iso: string) =>
		new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'medium' });
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Backups guardados</Card.Title>
		<Card.Description>
			Banco da QuadrinhoTeca (catálogo, fila, tarefas e configurações). Os quadrinhos não entram no backup.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-3">
		{#if list.length === 0}
			<p class="text-muted-foreground text-sm">Nenhum backup ainda.</p>
		{:else}
			<ul class="divide-y rounded-md border text-sm">
				{#each list as b (b.name)}
					<li class="flex items-center justify-between gap-2 px-3 py-2">
						<span>{when(b.createdAt)} <span class="text-muted-foreground">· {megabytes(b.size)}</span></span>
						<Button variant="ghost" size="icon-sm" href={api.backupUrl(b.name)} download={b.name} aria-label="Baixar {b.name}">
							<DownloadIcon />
						</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
	<Card.Footer class="flex flex-wrap gap-2">
		<Button type="button" variant="outline" size="sm" disabled={busy} onclick={createNow}>
			<DatabaseBackupIcon /> Fazer backup agora
		</Button>
		<Button type="button" variant="outline" size="sm" disabled={busy} onclick={() => input.click()}>
			<UploadIcon /> Restaurar de um arquivo…
		</Button>
		<input
			bind:this={input}
			type="file"
			accept=".zip,application/zip"
			class="hidden"
			onchange={(e) => (pending = e.currentTarget.files?.[0] ?? null)}
		/>
	</Card.Footer>
</Card.Root>

<AlertDialog.Root open={pending !== null} onOpenChange={(o) => !o && (pending = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Restaurar “{pending?.name}”?</AlertDialog.Title>
			<AlertDialog.Description>
				O catálogo, a fila, as tarefas e as configurações atuais serão substituídos pelos do backup. Antes, a
				QuadrinhoTeca guarda um backup do estado atual para você poder voltar atrás. Os arquivos dos
				quadrinhos não são alterados.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancelar</AlertDialog.Cancel>
			<AlertDialog.Action onclick={restore}>Restaurar</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
