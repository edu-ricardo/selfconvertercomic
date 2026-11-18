<script lang="ts">
	import { thumbUrl } from '$lib/api';
	import BookImageIcon from '@lucide/svelte/icons/book-image';

	let { id, mtime, alt, class: cls = '' }: { id: number; mtime?: number; alt: string; class?: string } =
		$props();
	let failed = $state(false);
</script>

<div class="bg-muted relative aspect-[2/3] overflow-hidden rounded-md {cls}">
	{#if failed}
		<div class="text-muted-foreground absolute inset-0 flex items-center justify-center">
			<BookImageIcon class="size-8 opacity-50" />
		</div>
	{:else}
		<img
			src={thumbUrl(id, mtime)}
			{alt}
			loading="lazy"
			decoding="async"
			class="h-full w-full object-cover"
			onerror={() => (failed = true)}
		/>
	{/if}
</div>
