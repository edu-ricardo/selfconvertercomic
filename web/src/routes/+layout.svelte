<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { ModeWatcher, toggleMode } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner';
	import { Button } from '$lib/components/ui/button';
	import { api } from '$lib/api';
	import { taskCount, refreshTaskCount } from '$lib/task-count.svelte';
	import LibraryIcon from '@lucide/svelte/icons/library-big';
	import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
	import ListOrderedIcon from '@lucide/svelte/icons/list-ordered';
	import FolderTreeIcon from '@lucide/svelte/icons/folder-tree';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import MenuIcon from '@lucide/svelte/icons/menu';
	import XIcon from '@lucide/svelte/icons/x';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';

	let { children } = $props();

	const nav = [
		{ href: '/catalogo', label: 'Catálogo', icon: LibraryIcon },
		{ href: '/bibliotecas', label: 'Bibliotecas', icon: FolderOpenIcon },
		{ href: '/organizar', label: 'Organizar', icon: FolderTreeIcon },
		{ href: '/duplicados', label: 'Duplicados', icon: CopyIcon },
		{ href: '/tarefas', label: 'Tarefas', icon: ListChecksIcon },
		{ href: '/fila', label: 'Fila', icon: ListOrderedIcon },
		{ href: '/configuracoes', label: 'Configurações', icon: SettingsIcon }
	];

	let menuOpen = $state(false);
	let version = $state('');

	$effect(() => {
		api
			.health()
			.then((h) => (version = h.version))
			.catch(() => (version = 'offline'));
	});

	$effect(() => {
		refreshTaskCount();
		const t = setInterval(refreshTaskCount, 15000);
		return () => clearInterval(t);
	});

	const isActive = (href: string) => page.url.pathname.startsWith(href);
</script>

<ModeWatcher />
<Toaster richColors position="top-right" />

<div class="bg-background text-foreground flex min-h-screen">
	<!-- Fundo escurecido do menu no celular -->
	{#if menuOpen}
		<button
			class="fixed inset-0 z-30 bg-black/40 md:hidden"
			aria-label="Fechar menu"
			onclick={() => (menuOpen = false)}
		></button>
	{/if}

	<aside
		class={[
			'bg-sidebar text-sidebar-foreground fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r transition-transform md:sticky md:top-0 md:h-screen md:translate-x-0',
			menuOpen ? 'translate-x-0' : '-translate-x-full'
		]}
	>
		<div class="flex h-14 items-center justify-between px-4">
			<a href="/catalogo" class="text-lg font-semibold tracking-tight">QuadrinhoTeca</a>
			<Button
				variant="ghost"
				size="icon"
				class="md:hidden"
				aria-label="Fechar menu"
				onclick={() => (menuOpen = false)}
			>
				<XIcon />
			</Button>
		</div>
		<nav class="flex flex-1 flex-col gap-1 px-2">
			{#each nav as item (item.href)}
				<a
					href={item.href}
					onclick={() => (menuOpen = false)}
					aria-current={isActive(item.href) ? 'page' : undefined}
					class="hover:bg-sidebar-accent aria-[current=page]:bg-sidebar-accent aria-[current=page]:text-sidebar-accent-foreground flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors"
				>
					<item.icon class="size-4" />
					{item.label}
					{#if item.href === '/tarefas' && taskCount.open > 0}
						<span
							class="bg-destructive ml-auto rounded-full px-1.5 text-[11px] leading-5 font-semibold text-white"
							aria-label="{taskCount.open} tarefas pendentes">{taskCount.open}</span
						>
					{/if}
				</a>
			{/each}
		</nav>
		<div class="text-muted-foreground flex items-center justify-between px-4 py-3 text-xs">
			<span>versão {version}</span>
			<Button variant="ghost" size="icon" aria-label="Alternar tema" onclick={toggleMode}>
				<SunIcon class="dark:hidden" />
				<MoonIcon class="hidden dark:block" />
			</Button>
		</div>
	</aside>

	<div class="flex min-w-0 flex-1 flex-col">
		<header class="flex h-14 items-center gap-2 border-b px-4 md:hidden">
			<Button variant="ghost" size="icon" aria-label="Abrir menu" onclick={() => (menuOpen = true)}>
				<MenuIcon />
			</Button>
			<span class="font-semibold">QuadrinhoTeca</span>
		</header>
		<main class="mx-auto w-full max-w-6xl flex-1 p-4 md:p-8">
			{@render children()}
		</main>
	</div>
</div>
