<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { api, ApiError, type Settings } from '$lib/api';
	import SendIcon from '@lucide/svelte/icons/send';

	let s = $state<Settings | null>(null);
	let errors = $state<Record<string, string>>({});
	let saving = $state(false);
	let loadError = $state('');

	const originalActions = [
		{ value: 'keep', label: 'Manter no lugar' },
		{ value: 'archive', label: 'Mover para pasta de arquivo' },
		{ value: 'delete', label: 'Apagar' }
	];

	$effect(() => {
		api
			.getSettings()
			.then((v) => (s = v))
			.catch((e) => (loadError = e.message));
	});

	let testing = $state(false);

	// O teste usa as configurações salvas no servidor: salva antes de enviar.
	async function testNtfy() {
		if (!s) return;
		testing = true;
		try {
			s = await api.putSettings($state.snapshot(s));
			errors = {};
			await api.testNotification();
			toast.success('Notificação de teste enviada; confira o celular');
		} catch (e) {
			errors = e instanceof ApiError ? e.fields : {};
			toast.error(e instanceof Error ? e.message : 'Falha ao enviar');
		} finally {
			testing = false;
		}
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!s) return;
		saving = true;
		try {
			s = await api.putSettings($state.snapshot(s));
			errors = {};
			toast.success('Configurações salvas');
		} catch (e) {
			errors = e instanceof ApiError ? e.fields : {};
			toast.error(e instanceof Error ? e.message : 'Falha ao salvar');
		} finally {
			saving = false;
		}
	}
</script>

{#snippet field(id: string, hint?: string)}
	{#if errors[id]}
		<p class="text-destructive text-xs">{errors[id]}</p>
	{:else if hint}
		<p class="text-muted-foreground text-xs">{hint}</p>
	{/if}
{/snippet}

<PageHeader title="Configurações" description="Ajustes gerais da QuadrinhoTeca." />

{#if loadError}
	<p class="text-destructive">Não foi possível carregar: {loadError}</p>
{:else if !s}
	<p class="text-muted-foreground">Carregando…</p>
{:else}
	<form class="grid gap-6" onsubmit={save}>
		<Card.Root>
			<Card.Header>
				<Card.Title>Conversão</Card.Title>
				<Card.Description>PDF → CBZ, sem recompressão das páginas.</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="flex items-center justify-between gap-4 sm:col-span-2">
					<Label for="autoConvert">Converter automaticamente PDFs novos</Label>
					<Switch id="autoConvert" bind:checked={s.conversion.autoConvert} />
				</div>
				<div class="grid gap-1.5">
					<Label for="stability">Esperar o arquivo ficar estável (min)</Label>
					<Input id="stability" type="number" min="0" bind:value={s.conversion.stabilityMinutes} />
					{@render field('conversion.stabilityMinutes', 'Evita converter arquivo ainda sendo copiado.')}
				</div>
				<div class="grid gap-1.5">
					<Label>Depois de converter, o PDF original</Label>
					<Select.Root type="single" bind:value={s.conversion.originalAction}>
						<Select.Trigger class="w-full">
							{originalActions.find((o) => o.value === s?.conversion.originalAction)?.label}
						</Select.Trigger>
						<Select.Content>
							{#each originalActions as o (o.value)}
								<Select.Item value={o.value} label={o.label} />
							{/each}
						</Select.Content>
					</Select.Root>
					{@render field('conversion.originalAction')}
				</div>
				{#if s.conversion.originalAction === 'archive'}
					<div class="grid gap-1.5 sm:col-span-2">
						<Label for="archiveDir">Pasta de arquivo</Label>
						<Input id="archiveDir" placeholder="/external/Untitled/pdf-arquivados" bind:value={s.conversion.archiveDir} />
						{@render field('conversion.archiveDir')}
					</div>
				{/if}
				<div class="grid gap-1.5">
					<Label for="workers">Conversões simultâneas</Label>
					<Input id="workers" type="number" min="1" bind:value={s.conversion.workers} />
					{@render field('conversion.workers')}
				</div>
				<div class="grid gap-1.5">
					<Label for="turboWorkers">Modo turbo: conversões simultâneas</Label>
					<Input id="turboWorkers" type="number" min="1" bind:value={s.conversion.turboWorkers} />
					{@render field('conversion.turboWorkers')}
				</div>
				<div class="grid gap-1.5">
					<Label for="turboHours">Modo turbo: desliga sozinho após (h)</Label>
					<Input id="turboHours" type="number" min="1" bind:value={s.conversion.turboHours} />
					{@render field('conversion.turboHours')}
				</div>
				<div class="grid gap-1.5">
					<Label for="rescan">Revarredura das bibliotecas a cada (h)</Label>
					<Input id="rescan" type="number" min="1" bind:value={s.scan.rescanHours} />
					{@render field('scan.rescanHours')}
				</div>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Notificações (ntfy)</Card.Title>
				<Card.Description>Deixe a URL vazia para desligar.</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="grid gap-1.5">
					<Label for="ntfyUrl">URL do servidor</Label>
					<Input id="ntfyUrl" placeholder="http://umbrel.local:2586" bind:value={s.notifications.ntfyUrl} />
					{@render field('notifications.ntfyUrl')}
				</div>
				<div class="grid gap-1.5">
					<Label for="ntfyTopic">Tópico</Label>
					<Input id="ntfyTopic" placeholder="quadrinhoteca" bind:value={s.notifications.ntfyTopic} />
					{@render field('notifications.ntfyTopic')}
				</div>
				<div class="flex items-center justify-between gap-4">
					<Label for="onFinish">Quando uma conversão terminar</Label>
					<Switch id="onFinish" bind:checked={s.notifications.onFinish} />
				</div>
				<div class="flex items-center justify-between gap-4">
					<Label for="onError">Quando der erro</Label>
					<Switch id="onError" bind:checked={s.notifications.onError} />
				</div>
				<div class="flex items-center justify-between gap-4">
					<Label for="onTasks">Quando houver tarefas de ajuste</Label>
					<Switch id="onTasks" bind:checked={s.notifications.onTasks} />
				</div>
				<p class="text-muted-foreground text-xs sm:col-span-2">
					Um resumo único é enviado quando a fila de conversão esvazia (sem um aviso por arquivo).
				</p>
			</Card.Content>
			<Card.Footer>
				<Button type="button" variant="outline" size="sm" disabled={testing || !s.notifications.ntfyUrl} onclick={testNtfy}>
					<SendIcon /> {testing ? 'Enviando…' : 'Salvar e enviar teste'}
				</Button>
			</Card.Footer>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Leitores</Card.Title>
				<Card.Description>Endereço base usado nos links "abrir no leitor".</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="grid gap-1.5">
					<Label for="kavita">Kavita</Label>
					<Input id="kavita" placeholder="http://umbrel.local:8798" bind:value={s.readers.kavitaUrl} />
					{@render field('readers.kavitaUrl')}
				</div>
				<div class="grid gap-1.5">
					<Label for="bookorbit">BookOrbit</Label>
					<Input id="bookorbit" placeholder="http://umbrel.local:8746" bind:value={s.readers.bookOrbitUrl} />
					{@render field('readers.bookOrbitUrl')}
				</div>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Metadados</Card.Title>
				<Card.Description>Usados na entrega 2 (Guia dos Quadrinhos e ComicVine).</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="grid gap-1.5 sm:col-span-2">
					<Label for="cv">Chave de API do ComicVine</Label>
					<Input id="cv" type="password" autocomplete="off" bind:value={s.metadata.comicVineApiKey} />
				</div>
				<div class="grid gap-1.5">
					<Label for="guiaRate">Guia dos Quadrinhos: segundos entre requisições</Label>
					<Input id="guiaRate" type="number" min="1" bind:value={s.metadata.guiaRateSeconds} />
					{@render field('metadata.guiaRateSeconds')}
				</div>
				<div class="grid gap-1.5">
					<Label for="cacheDays">Guardar páginas consultadas por (dias)</Label>
					<Input id="cacheDays" type="number" min="1" bind:value={s.metadata.cacheDays} />
					{@render field('metadata.cacheDays')}
				</div>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Backup</Card.Title>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="flex items-center justify-between gap-4">
					<Label for="daily">Backup diário automático</Label>
					<Switch id="daily" bind:checked={s.backup.daily} />
				</div>
				<div class="grid gap-1.5">
					<Label for="keep">Cópias mantidas</Label>
					<Input id="keep" type="number" min="1" bind:value={s.backup.keep} />
					{@render field('backup.keep')}
				</div>
			</Card.Content>
		</Card.Root>

		<div class="bg-background/80 sticky bottom-0 flex justify-end border-t py-3 backdrop-blur">
			<Button type="submit" disabled={saving}>{saving ? 'Salvando…' : 'Salvar'}</Button>
		</div>
	</form>
{/if}
