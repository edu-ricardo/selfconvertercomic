# CLAUDE.md

## Projeto

**QuadrinhoTeca** (repositório `selfconvertercomic`) — aplicação self-hosted para o homelab (Umbrel) que:

1. Converte quadrinhos em PDF para CBZ/CBR.
2. Organiza a biblioteca de quadrinhos.
3. (Secundário) Busca metadados em português para os quadrinhos (hoje consumidos pelo Bookorbit).

Requisitos não funcionais: leve e rápida, empacotada como app Umbrel (Docker) para uma loja própria,
frontend moderno e com visual padronizado.

Status: **levantamento de requisitos**. Decisões e respostas ficam em [docs/requisitos.md](docs/requisitos.md);
opções de stack em [docs/stack.md](docs/stack.md). Não escrever código de produto antes de os requisitos
do MVP estarem fechados nesses documentos.

## Convenções

- Idioma: documentação, commits e UI em português (pt-BR); identificadores de código em inglês.
- Commits pequenos e focados, mensagem no imperativo.
- Toda decisão de requisito/arquitetura vai para `docs/` antes de ir para o código.

---

## Diretrizes de desenvolvimento

Adaptado de [multica-ai/andrej-karpathy-skills](https://github.com/multica-ai/andrej-karpathy-skills) (MIT).
A skill completa está em [.claude/skills/karpathy-guidelines/SKILL.md](.claude/skills/karpathy-guidelines/SKILL.md).

**Tradeoff:** estas diretrizes priorizam cautela sobre velocidade. Em tarefas triviais, use bom senso.

### 1. Pensar antes de codar

**Não presuma. Não esconda dúvidas. Exponha os tradeoffs.**

- Declare as suposições explicitamente. Se houver incerteza, pergunte.
- Se houver múltiplas interpretações, apresente-as — não escolha em silêncio.
- Se existir uma abordagem mais simples, diga. Discorde quando fizer sentido.
- Se algo estiver confuso, pare, nomeie o que está confuso e pergunte.

### 2. Simplicidade primeiro

**O mínimo de código que resolve o problema. Nada especulativo.**

- Nenhuma funcionalidade além do pedido.
- Nenhuma abstração para código de uso único.
- Nenhuma "flexibilidade" ou "configurabilidade" não solicitada.
- Nenhum tratamento de erro para cenários impossíveis.
- Se escreveu 200 linhas e poderiam ser 50, reescreva.

Pergunta-teste: "Um engenheiro sênior diria que isso está complicado demais?" Se sim, simplifique.

### 3. Mudanças cirúrgicas

**Toque só no necessário. Limpe só a sua própria bagunça.**

- Não "melhore" código, comentários ou formatação adjacentes.
- Não refatore o que não está quebrado. Siga o estilo existente.
- Código morto não relacionado: mencione, não apague.
- Remova imports/variáveis/funções que a SUA mudança deixou sem uso.

Teste: cada linha alterada deve se ligar diretamente ao pedido.

### 4. Execução guiada por objetivos

**Defina critérios de sucesso. Itere até verificar.**

- "Adicionar validação" → "Escrever testes para entradas inválidas e fazê-los passar".
- "Corrigir o bug" → "Escrever um teste que reproduz o bug e fazê-lo passar".
- "Refatorar X" → "Garantir que os testes passam antes e depois".

Para tarefas com várias etapas, declare um plano curto:

```
1. [Etapa] → verificar: [checagem]
2. [Etapa] → verificar: [checagem]
```
