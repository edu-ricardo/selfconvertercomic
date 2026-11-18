<script lang="ts">
	import { toast } from 'svelte-sonner';
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { api, type Task, type TaskStatus } from '$lib/api';
	import { refreshTaskCount } from '$lib/task-count.svelte';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';

	const tabs: { value: TaskStatus; label: string }[] = [
		{ value: 'open', label: 'Pendentes' },
		{ value: 'resolved', label: 'Resolvidas' },
		{ value: 'dismissed', label: 'Ignoradas' }
	];

	let filter = $state<TaskStatus>('open');
	let list = $state<Task[] | null>(null);
	let open = $state(0);
	let error = $state('');

	async function load() {
		try {
			const r = await api.listTasks(filter);
			list = r.tasks;
			open = r.open;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	$effect(() => {
		filter;
		load();
		const t = setInterval(load, 5000);
		return () => clearInterval(t);
	});

	async function act(fn: () => Promise<unknown>, ok: string) {
		try {
			await fn();
			toast.success(ok);
		} catch (e) {
			toast.error(e instanceof Error ? e.message : 'Falha');
		}
		await load();
		refreshTaskCount();
	}

	const time = (iso: string | null) =>
		iso ? new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : '';
	const folder = (p: string) => p.split('/').slice(0, -1).join('/');
</script>

<PageHeader
	title="Tarefas"
	description={open ? `${open} aguardando você. Resolva quando tiver tempo.` : 'O que precisa da sua confirmação ou ajuste.'}
/>

<div class="mb-4 flex flex-wrap gap-2" role="tablist">
	{#each tabs as t (t.value)}
		<Button
			role="tab"
			aria-selected={filter === t.value}
			variant={filter === t.value ? 'default' : 'outline'}
			size="sm"
			onclick={() => {
				filter = t.value;
				list = null;
			}}
		>
			{t.label}
			{#if t.value === 'open'}<span class="opacity-70">{open}</span>{/if}
		</Button>
	{/each}
</div>

{#if error}
	<p class="text-destructive">Não foi possível carregar: {error}</p>
{:else if !list}
	<p class="text-muted-foreground">Carregando…</p>
{:else if list.length === 0}
	<EmptyState
		icon={ListChecksIcon}
		title={filter === 'open' ? 'Nada pendente' : 'Nada por aqui'}
		text={filter === 'open'
			? 'Erros de conversão aparecem aqui. Na entrega 2, também as confirmações de metadados e de organização.'
			: 'Nenhuma tarefa neste estado.'}
	/>
{:else}
	<ul class="grid gap-2">
		{#each list as t (t.id)}
			<li class="bg-card grid gap-2 rounded-lg border p-3">
				<div class="flex flex-wrap items-start justify-between gap-2">
					<div class="min-w-0">
						<p class="font-medium break-words">{t.title}</p>
						<p class="text-muted-foreground truncate text-xs" title={t.relPath}>
							{t.libraryName}{folder(t.relPath) ? ' / ' + folder(t.relPath) : ''} · {time(t.createdAt)}
						</p>
					</div>
					{#if t.status === 'open'}
						<div class="flex gap-1">
							{#if t.kind === 'conversion_error' && t.title.startsWith('Erro')}
								<Button size="sm" variant="outline" onclick={() => act(() => api.retryTask(t.id), 'De volta à fila de conversão')}>
									<RotateCcwIcon /> Tentar de novo
								</Button>
							{/if}
							<Button size="sm" variant="ghost" onclick={() => act(() => api.dismissTask(t.id), 'Tarefa ignorada')}>
								<EyeOffIcon /> Ignorar
							</Button>
						</div>
					{:else}
						<Badge variant="secondary">{t.status === 'resolved' ? 'Resolvida' : 'Ignorada'} {time(t.resolvedAt)}</Badge>
					{/if}
				</div>
				{#if t.detail}
					<p class="bg-muted rounded px-2 py-1 font-mono text-xs break-words">{t.detail}</p>
				{/if}
			</li>
		{/each}
	</ul>
{/if}
