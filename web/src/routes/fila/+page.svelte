<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { api, type Job, type JobFilter, type JobStatus, type Turbo } from '$lib/api';
	import ListOrderedIcon from '@lucide/svelte/icons/list-ordered';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import XIcon from '@lucide/svelte/icons/x';
	import ZapIcon from '@lucide/svelte/icons/zap';

	const tabs: { value: JobFilter; label: string; count: (c: Record<JobStatus, number>) => number }[] = [
		{ value: 'active', label: 'Na fila', count: (c) => c.running + c.queued },
		{ value: 'failed', label: 'Com erro', count: (c) => c.failed },
		{ value: 'done', label: 'Concluídas', count: (c) => c.done },
		{ value: 'cancelled', label: 'Canceladas', count: (c) => c.cancelled }
	];

	let filter = $state<JobFilter>('active');
	let jobs = $state<Job[] | null>(null);
	let counts = $state<Record<JobStatus, number>>({ queued: 0, running: 0, done: 0, failed: 0, cancelled: 0 });
	let turbo = $state<Turbo>({ enabled: false, until: null });
	let loadError = $state('');

	async function load() {
		try {
			const r = await api.listJobs(filter);
			jobs = r.jobs;
			counts = r.counts;
			turbo = r.turbo;
			loadError = '';
		} catch (e) {
			loadError = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		filter; // recarrega ao trocar de aba
		load();
		const t = setInterval(load, 2000);
		return () => clearInterval(t);
	});

	async function act(fn: () => Promise<unknown>, ok: string) {
		try {
			await fn();
			toast.success(ok);
			await load();
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		}
	}

	async function toggleTurbo(enabled: boolean) {
		try {
			turbo = await api.setTurbo(enabled);
			toast.success(enabled ? 'Modo turbo ligado' : 'Modo turbo desligado');
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		}
	}

	const statusLabel: Record<JobStatus, string> = {
		queued: 'Aguardando',
		running: 'Convertendo',
		done: 'Concluída',
		failed: 'Erro',
		cancelled: 'Cancelada'
	};
	const statusVariant = (s: JobStatus) =>
		s === 'failed' ? 'destructive' : s === 'running' ? 'default' : s === 'done' ? 'secondary' : 'outline';
	const fileName = (p: string) => p.split('/').pop() ?? p;
	const folder = (p: string) => p.split('/').slice(0, -1).join('/');
	const mb = (n: number) => (n / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + ' MB';
	const time = (iso: string | null) =>
		iso ? new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : '';
</script>

<PageHeader title="Fila" description="Conversões PDF → CBZ: em andamento, aguardando e histórico." />

<Card.Root class="mb-6">
	<Card.Content class="flex items-center justify-between gap-4">
		<div class="flex min-w-0 flex-1 items-center gap-3">
			<ZapIcon class={turbo.enabled ? 'size-5 text-amber-500' : 'text-muted-foreground size-5'} />
			<div>
				<Label for="turbo" class="text-base">Modo turbo</Label>
				<p class="text-muted-foreground text-xs">
					{turbo.enabled
						? `Mais conversões ao mesmo tempo até ${time(turbo.until)}.`
						: 'Converte mais arquivos ao mesmo tempo por algumas horas (ajuste em Configurações).'}
				</p>
			</div>
		</div>
		<Switch id="turbo" checked={turbo.enabled} onCheckedChange={toggleTurbo} />
	</Card.Content>
</Card.Root>

<div class="mb-4 flex flex-wrap gap-2" role="tablist">
	{#each tabs as t (t.value)}
		<Button
			role="tab"
			aria-selected={filter === t.value}
			variant={filter === t.value ? 'default' : 'outline'}
			size="sm"
			onclick={() => {
				filter = t.value;
				jobs = null;
			}}
		>
			{t.label}
			<span class="opacity-70">{t.count(counts)}</span>
		</Button>
	{/each}
</div>

{#if loadError}
	<p class="text-destructive">Não foi possível carregar: {loadError}</p>
{:else if !jobs}
	<p class="text-muted-foreground">Carregando…</p>
{:else if jobs.length === 0}
	<EmptyState
		icon={ListOrderedIcon}
		title="Nada por aqui"
		text={filter === 'active'
			? 'PDFs novos entram na fila sozinhos; ou use "Converter PDFs" em Bibliotecas.'
			: 'Nenhuma conversão neste estado.'}
	/>
{:else}
	<ul class="grid gap-2">
		{#each jobs as j (j.id)}
			<li class="bg-card grid gap-2 rounded-lg border p-3">
				<div class="flex flex-wrap items-start justify-between gap-2">
					<div class="min-w-0">
						<p class="truncate font-medium" title={j.relPath}>{fileName(j.relPath)}</p>
						<p class="text-muted-foreground truncate text-xs">
							{j.libraryName}{folder(j.relPath) ? ' / ' + folder(j.relPath) : ''} · {mb(j.srcSize)}
							{j.auto ? '· automática' : '· manual'}
						</p>
					</div>
					<div class="flex items-center gap-1">
						<Badge variant={statusVariant(j.status)}>{statusLabel[j.status]}</Badge>
						{#if j.status === 'queued' || j.status === 'running'}
							<Button variant="ghost" size="icon-sm" aria-label="Cancelar"
								onclick={() => act(() => api.cancelJob(j.id), 'Cancelada')}>
								<XIcon />
							</Button>
						{:else if j.status === 'failed' || j.status === 'cancelled'}
							<Button variant="ghost" size="icon-sm" aria-label="Tentar de novo"
								onclick={() => act(() => api.retryJob(j.id), 'De volta à fila')}>
								<RotateCcwIcon />
							</Button>
						{/if}
					</div>
				</div>
				{#if j.status === 'running'}
					<div class="bg-muted h-1.5 overflow-hidden rounded-full" role="progressbar"
						aria-valuenow={j.progress} aria-valuemin={0} aria-valuemax={100}>
						<div class="bg-primary h-full transition-all" style="width: {j.progress}%"></div>
					</div>
				{/if}
				{#if j.status === 'done'}
					<p class="text-muted-foreground text-xs">
						{j.pages} páginas ({j.extracted} sem recompressão{j.rendered ? `, ${j.rendered} renderizadas` : ''})
						· {time(j.finishedAt)}
					</p>
				{/if}
				{#if j.error}<p class={j.status === 'done' ? 'text-xs text-amber-600' : 'text-destructive text-xs'}>{j.error}</p>{/if}
			</li>
		{/each}
	</ul>
{/if}
