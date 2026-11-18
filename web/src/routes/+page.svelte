<script lang="ts">
	let version = $state('…');

	$effect(() => {
		fetch('/api/health')
			.then((r) => r.json())
			.then((b) => (version = b.version))
			.catch(() => (version = 'API indisponível'));
	});
</script>

<main class="flex min-h-screen flex-col items-center justify-center gap-2 bg-zinc-50 p-4 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
	<h1 class="text-3xl font-semibold">QuadrinhoTeca</h1>
	<p class="text-sm text-zinc-500">versão {version}</p>
</main>
