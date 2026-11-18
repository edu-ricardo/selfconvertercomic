<script lang="ts">
	import type { Component } from 'svelte';

	let { icon: Icon, title, text }: { icon: Component; title: string; text: string } = $props();
</script>

<div
	class="text-muted-foreground flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-12 text-center"
>
	<Icon class="size-10 opacity-60" />
	<p class="text-foreground font-medium">{title}</p>
	<p class="max-w-sm text-sm">{text}</p>
</div>
