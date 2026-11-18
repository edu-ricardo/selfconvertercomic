<script lang="ts">
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import LibraryIcon from '@lucide/svelte/icons/library-big';
</script>

<PageHeader title="Catálogo" description="Seus quadrinhos, agrupados por série." />
<EmptyState
	icon={LibraryIcon}
	title="Nenhuma biblioteca ainda"
	text="Cadastre uma pasta em Bibliotecas. A grade de capas chega no marco M4."
/>
