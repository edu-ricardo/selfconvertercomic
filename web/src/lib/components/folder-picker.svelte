<script lang="ts">
	import { api, type DirListing } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';

	/** Pasta escolhida ("" enquanto o usuário está na lista de raízes). */
	let { value = $bindable('') }: { value?: string } = $props();

	let listing = $state<DirListing>({ path: '', parent: '', dirs: [] });
	let error = $state('');
	let loading = $state(false);

	async function open(path: string) {
		loading = true;
		try {
			listing = await api.browse(path);
			value = listing.path;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		open('');
	});
</script>

<div class="rounded-md border">
	<div class="bg-muted/50 flex items-center gap-2 border-b px-3 py-2 text-sm">
		<Button
			variant="ghost"
			size="icon-sm"
			aria-label="Pasta acima"
			disabled={!listing.path || loading}
			onclick={() => open(listing.parent)}
		>
			<ArrowUpIcon />
		</Button>
		<span class="truncate font-mono text-xs" title={listing.path}>
			{listing.path || 'Discos e armazenamento'}
		</span>
	</div>
	<ul class="max-h-64 overflow-y-auto p-1" aria-busy={loading}>
		{#each listing.dirs as d (d.path)}
			<li>
				<button
					type="button"
					class="hover:bg-accent flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm"
					onclick={() => open(d.path)}
				>
					{#if listing.path}
						<FolderIcon class="text-muted-foreground size-4 shrink-0" />
					{:else}
						<HardDriveIcon class="text-muted-foreground size-4 shrink-0" />
					{/if}
					<span class="truncate">{d.name}</span>
				</button>
			</li>
		{:else}
			<li class="text-muted-foreground px-2 py-6 text-center text-sm">
				{listing.path ? 'Sem subpastas.' : 'Nenhum disco disponível.'}
			</li>
		{/each}
	</ul>
	{#if error}<p class="text-destructive border-t px-3 py-2 text-xs">{error}</p>{/if}
</div>
