<script lang="ts">
	import * as Select from '$lib/components/ui/select';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { api, type CatalogFilter, type Library } from '$lib/api';
	import { fileStatusLabel, formatLabel } from '$lib/status';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';

	/** Filtros atuais; onchange é chamado a cada mudança (a busca espera o usuário parar de digitar). */
	let {
		filter,
		onchange,
		showLibrary = true
	}: { filter: CatalogFilter; onchange: (f: CatalogFilter) => void; showLibrary?: boolean } = $props();

	let libraries = $state<Library[]>([]);
	let publishers = $state<string[]>([]);
	let q = $state('');
	// Acompanha a URL (voltar/avançar, "Limpar"); enquanto digita, a URL só muda depois da pausa.
	$effect(() => {
		q = filter.q ?? '';
	});

	$effect(() => {
		if (showLibrary) api.listLibraries().then((r) => (libraries = r.libraries)).catch(() => {});
	});
	$effect(() => {
		api.publishers(filter.library).then((p) => (publishers = p)).catch(() => {});
	});

	let timer: ReturnType<typeof setTimeout>;
	function search(value: string) {
		q = value;
		clearTimeout(timer);
		timer = setTimeout(() => onchange({ ...filter, q: value }), 300);
	}

	const set = (patch: Partial<CatalogFilter>) => onchange({ ...filter, ...patch });
	const ALL = '_';
	// Na página da série a biblioteca é fixa: não conta como filtro "limpável".
	const active = $derived(
		!!(filter.q || (showLibrary && filter.library) || filter.publisher || filter.format || filter.status)
	);
</script>

<div class="mb-6 grid gap-2 sm:grid-cols-2 lg:grid-cols-[1fr_repeat(4,minmax(0,11rem))_auto]">
	<div class="relative sm:col-span-2 lg:col-span-1">
		<SearchIcon class="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
		<Input
			type="search"
			placeholder="Buscar série, título ou editora"
			class="pl-8"
			value={q}
			oninput={(e) => search(e.currentTarget.value)}
		/>
	</div>

	{#if showLibrary}
		<Select.Root
			type="single"
			value={filter.library ? String(filter.library) : ALL}
			onValueChange={(v) => set({ library: v === ALL ? undefined : Number(v), publisher: undefined })}
		>
			<Select.Trigger class="w-full">
				{libraries.find((l) => l.id === filter.library)?.name ?? 'Todas as bibliotecas'}
			</Select.Trigger>
			<Select.Content>
				<Select.Item value={ALL} label="Todas as bibliotecas" />
				{#each libraries as l (l.id)}<Select.Item value={String(l.id)} label={l.name} />{/each}
			</Select.Content>
		</Select.Root>
	{/if}

	<Select.Root
		type="single"
		value={filter.publisher || ALL}
		onValueChange={(v) => set({ publisher: v === ALL ? undefined : v })}
	>
		<Select.Trigger class="w-full">{filter.publisher || 'Todas as editoras'}</Select.Trigger>
		<Select.Content>
			<Select.Item value={ALL} label="Todas as editoras" />
			{#each publishers as p (p)}<Select.Item value={p} label={p} />{/each}
		</Select.Content>
	</Select.Root>

	<Select.Root
		type="single"
		value={filter.format || ALL}
		onValueChange={(v) => set({ format: v === ALL ? '' : (v as CatalogFilter['format']) })}
	>
		<Select.Trigger class="w-full">{filter.format ? formatLabel[filter.format] : 'Todos os formatos'}</Select.Trigger>
		<Select.Content>
			<Select.Item value={ALL} label="Todos os formatos" />
			{#each Object.entries(formatLabel) as [v, label] (v)}<Select.Item value={v} {label} />{/each}
		</Select.Content>
	</Select.Root>

	<Select.Root
		type="single"
		value={filter.status || ALL}
		onValueChange={(v) => set({ status: v === ALL ? '' : (v as CatalogFilter['status']) })}
	>
		<Select.Trigger class="w-full">{filter.status ? fileStatusLabel[filter.status] : 'Qualquer status'}</Select.Trigger>
		<Select.Content>
			<Select.Item value={ALL} label="Qualquer status" />
			{#each Object.entries(fileStatusLabel) as [v, label] (v)}<Select.Item value={v} {label} />{/each}
		</Select.Content>
	</Select.Root>

	{#if active}
		<Button
			variant="ghost"
			onclick={() => {
				q = '';
				onchange({});
			}}><XIcon /> Limpar</Button
		>
	{/if}
</div>
